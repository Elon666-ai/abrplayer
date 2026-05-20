package ws

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"vpublisher/tracer"
)

const (
	retryDelayMin      = 3 * time.Second
	retryDelayMax      = 60 * time.Second
	retryResetAfterRun = 30 * time.Second
	stallCheckInterval = 3 * time.Second
	stallTimeout       = 20 * time.Second
	sharedWorkerKey    = "__shared__"
)

var (
	windowsEncoderOnce   sync.Once
	windowsEncoderConfig ffmpegEncoderConfig
	dshowAudioProbeCache sync.Map
	fileAudioProbeCache  sync.Map
)

type fileAudioProbeResult struct {
	hasAudio bool
}

type FFmpegConfig struct {
	InputFile   string
	TargetURLs  []string
	VideoLayout string
	Outputs     []OutputProfile
}

type OutputProfile struct {
	Level           string
	AudioOnly       bool
	VideoCodec      string
	VideoBitrate    string
	VideoMaxrate    string
	PortraitWidth   int
	PortraitHeight  int
	LandscapeWidth  int
	LandscapeHeight int
	AudioCodec      string
	AudioBitrate    string
}

type DeviceList struct {
	Video []string
	Audio []string
}

type ffmpegEncoderConfig struct {
	Name         string
	H264Encoder  string
	HEVCEncoder  string
	HardwareArgs []string
}

type ffmpegWorker struct {
	targetURL  string
	targetURLs []string

	mu        sync.Mutex
	cmd       *exec.Cmd
	running   bool
	stopCh    chan struct{}
	doneCh    chan struct{}
	startedAt time.Time
	status    string

	lastPtsFnMs        atomic.Int64
	lastProgressUnixMs atomic.Int64
}

type FFmpegManager struct {
	mu      sync.Mutex
	cfg     FFmpegConfig
	workers map[string]*ffmpegWorker
	paused  map[string]bool
}

var publisherMgr = &FFmpegManager{
	workers: make(map[string]*ffmpegWorker),
	paused:  make(map[string]bool),
}

func GetCurrentEncodingPtsFnMs() (int64, error) {
	publisherMgr.mu.Lock()
	workers := make([]*ffmpegWorker, 0, len(publisherMgr.workers))
	for _, w := range publisherMgr.workers {
		workers = append(workers, w)
	}
	publisherMgr.mu.Unlock()

	var anyRunning bool
	var maxPts int64
	for _, w := range workers {
		running, pts, startedAt, _ := w.snapshot()
		if !running {
			continue
		}
		anyRunning = true
		if pts <= 0 {
			pts = time.Since(startedAt).Milliseconds()
		}
		if pts > maxPts {
			maxPts = pts
		}
	}

	if !anyRunning {
		return 0, errors.New("publisher is not running")
	}
	if maxPts <= 0 {
		return 0, errors.New("publisher started but pts is unavailable")
	}
	return maxPts, nil
}

func GetPublisherSnapshot() map[string]bool {
	status := GetPublisherStatusSnapshot()
	out := make(map[string]bool, len(status))
	for target, value := range status {
		out[target] = value == "Running" || value == "Starting"
	}
	return out
}

func GetPublisherStatusSnapshot() map[string]string {
	publisherMgr.mu.Lock()
	cfg := publisherMgr.cfg
	targets := append([]string(nil), cfg.TargetURLs...)
	paused := make(map[string]bool, len(publisherMgr.paused))
	for target, isPaused := range publisherMgr.paused {
		paused[target] = isPaused
	}
	workers := make(map[string]*ffmpegWorker, len(publisherMgr.workers))
	for k, w := range publisherMgr.workers {
		workers[k] = w
	}
	publisherMgr.mu.Unlock()

	if shouldUseSharedFFmpeg(cfg.InputFile, targets) {
		out := make(map[string]string, len(targets))
		sharedStatus := "Stopped"
		if worker := workers[sharedWorkerKey]; worker != nil {
			_, _, _, sharedStatus = worker.snapshot()
		}
		for _, target := range targets {
			if paused[target] {
				out[target] = "Stopped"
				continue
			}
			out[target] = sharedStatus
		}
		return out
	}

	out := make(map[string]string, len(workers))
	for key, worker := range workers {
		if worker == nil {
			out[key] = "Stopped"
			continue
		}
		_, _, _, status := worker.snapshot()
		if paused[key] {
			status = "Stopped"
		}
		out[key] = status
	}
	return out
}

func ListInputDevices() (DeviceList, error) {
	logFFmpegDeviceProbeContext()
	video, audio, err := listDShowDevices()
	_ = tracer.LogInfo(tracer.ID_APP, "[DeviceProbe] parsed dshow devices: video=%d audio=%d videoNames=%s audioNames=%s",
		len(video), len(audio), strings.Join(video, " | "), strings.Join(audio, " | "))
	return DeviceList{Video: video, Audio: audio}, err
}

func InitFFmpegPublisher(cfg FFmpegConfig) {
	type pendingClose struct {
		stopCh chan struct{}
		doneCh chan struct{}
	}

	publisherMgr.mu.Lock()

	cfg.TargetURLs = uniqueNonEmpty(cfg.TargetURLs)
	publisherMgr.cfg = cfg
	useShared := shouldUseSharedFFmpeg(cfg.InputFile, cfg.TargetURLs)

	var pending []pendingClose

	if useShared {
		w, ok := publisherMgr.workers[sharedWorkerKey]
		if !ok || w == nil {
			w = &ffmpegWorker{targetURL: sharedWorkerKey}
			publisherMgr.workers[sharedWorkerKey] = w
		}
		w.targetURLs = append([]string(nil), cfg.TargetURLs...)

		for key, worker := range publisherMgr.workers {
			if key == sharedWorkerKey || worker == nil {
				continue
			}
			stopCh, doneCh, _ := worker.stopLocked()
			delete(publisherMgr.workers, key)
			pending = append(pending, pendingClose{stopCh: stopCh, doneCh: doneCh})
		}
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] shared process mode enabled for live input, outputs=%d", len(cfg.TargetURLs))
	} else {
		newSet := make(map[string]struct{}, len(cfg.TargetURLs))
		for _, url := range cfg.TargetURLs {
			newSet[url] = struct{}{}
			if _, ok := publisherMgr.workers[url]; !ok {
				publisherMgr.workers[url] = &ffmpegWorker{targetURL: url}
			}
		}

		for url, w := range publisherMgr.workers {
			if _, ok := newSet[url]; !ok {
				stopCh, doneCh, _ := w.stopLocked()
				publisherMgr.workers[url] = nil
				delete(publisherMgr.workers, url)
				pending = append(pending, pendingClose{stopCh: stopCh, doneCh: doneCh})
			}
		}
	}

	// Release the manager lock before signalling and waiting on workers, otherwise
	// we serialize the rest of the package behind a 5-second timeout per worker.
	publisherMgr.mu.Unlock()

	for _, p := range pending {
		if p.stopCh != nil {
			close(p.stopCh)
		}
	}
	for _, p := range pending {
		if p.doneCh == nil {
			continue
		}
		select {
		case <-p.doneCh:
		case <-time.After(5 * time.Second):
		}
	}
}

func StartFFmpegPublisher() error {
	publisherMgr.mu.Lock()
	defer publisherMgr.mu.Unlock()

	if publisherMgr.cfg.InputFile == "" || len(publisherMgr.cfg.TargetURLs) == 0 {
		return errors.New("ffmpeg config is not initialized")
	}

	if shouldUseSharedFFmpeg(publisherMgr.cfg.InputFile, publisherMgr.cfg.TargetURLs) {
		activeTargets := make([]string, 0, len(publisherMgr.cfg.TargetURLs))
		for _, url := range publisherMgr.cfg.TargetURLs {
			if publisherMgr.paused[url] {
				_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] start skipped for paused target=%s", url)
				continue
			}
			activeTargets = append(activeTargets, url)
		}
		if len(activeTargets) == 0 {
			_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] all shared-mode targets are paused, skip start")
			return nil
		}

		w := publisherMgr.workers[sharedWorkerKey]
		if w == nil {
			w = &ffmpegWorker{targetURL: sharedWorkerKey}
			publisherMgr.workers[sharedWorkerKey] = w
		}
		w.targetURLs = append([]string(nil), activeTargets...)
		if err := w.startLocked(publisherMgr.cfg.InputFile); err != nil {
			return err
		}
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] shared process started for %d targets", len(activeTargets))
		return nil
	}

	var firstErr error
	for _, url := range publisherMgr.cfg.TargetURLs {
		if publisherMgr.paused[url] {
			_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] start skipped for paused target=%s", url)
			continue
		}
		w := publisherMgr.workers[url]
		if w == nil {
			w = &ffmpegWorker{targetURL: url}
			publisherMgr.workers[url] = w
		}
		if err := w.startLocked(publisherMgr.cfg.InputFile); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] failed to start target=%s: %v", url, err)
		}
	}
	if firstErr == nil {
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] start requested for %d targets", len(publisherMgr.cfg.TargetURLs))
	}
	return firstErr
}

func StopFFmpegPublisher() error {
	publisherMgr.mu.Lock()
	workers := make([]*ffmpegWorker, 0, len(publisherMgr.workers))
	for _, w := range publisherMgr.workers {
		if w != nil {
			workers = append(workers, w)
		}
	}
	publisherMgr.mu.Unlock()

	for _, w := range workers {
		stopCh, doneCh, cmd := w.stopLocked()
		if stopCh != nil {
			close(stopCh)
		}
		if cmd != nil && cmd.Process != nil {
			if err := killProcess(cmd); err != nil {
				_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] failed to kill process target=%s: %v", w.targetURL, err)
			}
		}
		if doneCh != nil {
			select {
			case <-doneCh:
			case <-time.After(5 * time.Second):
				_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] stop timeout target=%s", w.targetURL)
			}
		}
	}
	return nil
}

func PauseFFmpegPublisher(reason string) error {
	publisherMgr.mu.Lock()
	for _, url := range publisherMgr.cfg.TargetURLs {
		publisherMgr.paused[url] = true
	}
	publisherMgr.mu.Unlock()

	_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] all targets paused: %s", reason)
	return StopFFmpegPublisher()
}

func ResumeFFmpegPublisher(reason string) error {
	publisherMgr.mu.Lock()
	for _, url := range publisherMgr.cfg.TargetURLs {
		delete(publisherMgr.paused, url)
	}
	publisherMgr.mu.Unlock()

	_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] all targets resumed: %s", reason)
	return StartFFmpegPublisher()
}

func PauseFFmpegPublisherByURL(targetURL, reason string) error {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return errors.New("publishUrl is empty")
	}

	publisherMgr.mu.Lock()
	matches := publisherMgr.resolveTargetsLocked(targetURL)
	useSharedMode := shouldUseSharedFFmpeg(publisherMgr.cfg.InputFile, publisherMgr.cfg.TargetURLs)
	for _, m := range matches {
		publisherMgr.paused[m] = true
	}
	workers := make([]*ffmpegWorker, 0, len(matches))
	for _, m := range matches {
		if w := publisherMgr.workers[m]; w != nil {
			workers = append(workers, w)
		}
	}
	publisherMgr.mu.Unlock()

	if len(matches) == 0 {
		return errors.New("target publishUrl is not configured")
	}
	if useSharedMode {
		_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] shared process mode: restart publisher to apply target pause")
		if err := StopFFmpegPublisher(); err != nil {
			return err
		}
		return StartFFmpegPublisher()
	}

	_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] target paused: input=%s matched=%s reason=%s",
		targetURL, strings.Join(matches, ","), reason)

	for _, w := range workers {
		stopCh, doneCh, cmd := w.stopLocked()
		if stopCh != nil {
			close(stopCh)
		}
		if cmd != nil && cmd.Process != nil {
			if err := killProcess(cmd); err != nil {
				return err
			}
		}
		if doneCh != nil {
			select {
			case <-doneCh:
			case <-time.After(5 * time.Second):
				_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] pause wait timeout target=%s", w.targetURL)
			}
		}
	}
	return nil
}

func ResumeFFmpegPublisherByURL(targetURL, reason string) error {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return errors.New("publishUrl is empty")
	}

	publisherMgr.mu.Lock()
	matches := publisherMgr.resolveTargetsLocked(targetURL)
	useSharedMode := shouldUseSharedFFmpeg(publisherMgr.cfg.InputFile, publisherMgr.cfg.TargetURLs)
	for _, m := range matches {
		delete(publisherMgr.paused, m)
	}
	workers := make([]*ffmpegWorker, 0, len(matches))
	for _, m := range matches {
		if w := publisherMgr.workers[m]; w != nil {
			workers = append(workers, w)
		}
	}
	inputFile := publisherMgr.cfg.InputFile
	publisherMgr.mu.Unlock()

	if len(matches) == 0 {
		return errors.New("target publishUrl is not configured")
	}
	if useSharedMode {
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] shared process mode: restart publisher to apply target resume")
		if err := StopFFmpegPublisher(); err != nil {
			return err
		}
		return StartFFmpegPublisher()
	}

	_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] target resumed: input=%s matched=%s reason=%s",
		targetURL, strings.Join(matches, ","), reason)

	var firstErr error
	for _, w := range workers {
		if err := w.startLocked(inputFile); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *FFmpegManager) resolveTargetsLocked(target string) []string {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}

	seen := make(map[string]struct{})
	add := func(v string) {
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
	}

	for _, configured := range m.cfg.TargetURLs {
		if configured == target {
			add(configured)
		}
	}
	if len(seen) > 0 {
		return mapKeysInOrder(m.cfg.TargetURLs, seen)
	}

	targetBase := normalizeURLBase(target)
	for _, configured := range m.cfg.TargetURLs {
		if normalizeURLBase(configured) == targetBase && targetBase != "" {
			add(configured)
			continue
		}
		if strings.HasPrefix(configured, target+"?") {
			add(configured)
		}
	}

	return mapKeysInOrder(m.cfg.TargetURLs, seen)
}

func normalizeURLBase(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + u.Path
}

func mapKeysInOrder(order []string, selected map[string]struct{}) []string {
	out := make([]string, 0, len(selected))
	for _, v := range order {
		if _, ok := selected[v]; ok {
			out = append(out, v)
		}
	}
	return out
}

func (w *ffmpegWorker) startLocked(inputFile string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if inputFile == "" {
		return errors.New("input file is empty")
	}
	if w.running {
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] start ignored, target already running: %s", w.targetURL)
		return nil
	}

	w.running = true
	w.status = "Starting"
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})
	go w.runLoop(inputFile)
	return nil
}

func (w *ffmpegWorker) stopLocked() (chan struct{}, chan struct{}, *exec.Cmd) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.running {
		return nil, nil, nil
	}
	return w.stopCh, w.doneCh, w.cmd
}

func (w *ffmpegWorker) snapshot() (bool, int64, time.Time, string) {
	w.mu.Lock()
	running := w.running
	startedAt := w.startedAt
	status := w.status
	w.mu.Unlock()
	if status == "" {
		if running {
			status = "Running"
		} else {
			status = "Stopped"
		}
	}
	return running, w.lastPtsFnMs.Load(), startedAt, status
}

func (w *ffmpegWorker) runLoop(inputFile string) {
	defer tracer.TryException()
	defer func() {
		w.mu.Lock()
		w.running = false
		w.cmd = nil
		w.startedAt = time.Time{}
		w.status = "Stopped"
		w.lastPtsFnMs.Store(0)
		if w.doneCh != nil {
			close(w.doneCh)
		}
		w.mu.Unlock()
	}()

	retryDelay := retryDelayMin
	retryAttempt := 0

	for {
		if w.isPaused() {
			_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] target paused, exit run loop: %s", w.targetURL)
			return
		}

		select {
		case <-w.stopCh:
			return
		default:
		}

		targets := append([]string(nil), w.targetURLs...)
		if len(targets) == 0 {
			targets = []string{w.targetURL}
		}
		layout := "portrait"
		outputs := []OutputProfile(nil)
		publisherMgr.mu.Lock()
		if strings.TrimSpace(publisherMgr.cfg.VideoLayout) != "" {
			layout = publisherMgr.cfg.VideoLayout
		}
		outputs = append(outputs, publisherMgr.cfg.Outputs...)
		publisherMgr.mu.Unlock()
		cfg := FFmpegConfig{InputFile: inputFile, TargetURLs: targets, VideoLayout: layout, Outputs: outputs}
		cmd, err := buildFFmpegCommand(cfg)
		if err != nil {
			retryAttempt++
			_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] build failed target=%s: %v", w.targetURL, err)
			if !w.waitRetry(retryAttempt, retryDelay, "build_failed") {
				return
			}
			retryDelay = minDuration(retryDelay*2, retryDelayMax)
			continue
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			retryAttempt++
			_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] stderr pipe failed target=%s: %v", w.targetURL, err)
			if !w.waitRetry(retryAttempt, retryDelay, "stderr_pipe_failed") {
				return
			}
			retryDelay = minDuration(retryDelay*2, retryDelayMax)
			continue
		}

		if err := cmd.Start(); err != nil {
			retryAttempt++
			_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] start failed target=%s: %v", w.targetURL, err)
			if !w.waitRetry(retryAttempt, retryDelay, "start_failed") {
				return
			}
			retryDelay = minDuration(retryDelay*2, retryDelayMax)
			continue
		}

		startedAt := time.Now()
		w.mu.Lock()
		w.cmd = cmd
		w.startedAt = startedAt
		w.status = "Running"
		w.lastPtsFnMs.Store(0)
		w.lastProgressUnixMs.Store(time.Now().UnixMilli())
		w.mu.Unlock()
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] process started target=%s pid=%d", w.targetURL, cmd.Process.Pid)

		parseDoneCh := make(chan struct{})
		go w.trackPtsFromFFmpegProgress(stderr, parseDoneCh)
		procDone := make(chan struct{})
		go w.monitorStallAndKill(cmd, startedAt, procDone)

		err = cmd.Wait()
		close(procDone)
		select {
		case <-parseDoneCh:
		case <-time.After(500 * time.Millisecond):
		}

		if err != nil {
			_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] process exited target=%s err=%v", w.targetURL, err)
		}

		if time.Since(startedAt) >= retryResetAfterRun {
			retryDelay = retryDelayMin
			retryAttempt = 0
		}

		if w.isPaused() {
			return
		}
		select {
		case <-w.stopCh:
			return
		default:
		}

		retryAttempt++
		if !w.waitRetry(retryAttempt, retryDelay, "process_exit") {
			return
		}
		retryDelay = minDuration(retryDelay*2, retryDelayMax)
	}
}

func (w *ffmpegWorker) isPaused() bool {
	publisherMgr.mu.Lock()
	defer publisherMgr.mu.Unlock()
	return publisherMgr.paused[w.targetURL]
}

func (w *ffmpegWorker) waitRetry(attempt int, delay time.Duration, reason string) bool {
	if delay < retryDelayMin {
		delay = retryDelayMin
	}
	if delay > retryDelayMax {
		delay = retryDelayMax
	}
	_ = tracer.LogInfo(tracer.ID_APP,
		"[FFMPEG] retry scheduled: target=%s attempt=%d reason=%s delay=%s",
		w.targetURL, attempt, reason, delay)
	w.mu.Lock()
	w.status = "Retrying"
	w.mu.Unlock()

	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-w.stopCh:
		return false
	case <-t.C:
		return true
	}
}

func (w *ffmpegWorker) monitorStallAndKill(cmd *exec.Cmd, startedAt time.Time, procDone <-chan struct{}) {
	ticker := time.NewTicker(stallCheckInterval)
	defer ticker.Stop()

	lastPts := w.lastPtsFnMs.Load()

	for {
		select {
		case <-procDone:
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
		}

		if time.Since(startedAt) < stallTimeout {
			continue
		}

		curPts := w.lastPtsFnMs.Load()
		if curPts > lastPts {
			lastPts = curPts
			continue
		}

		lastProgressAt := time.UnixMilli(w.lastProgressUnixMs.Load())
		if lastProgressAt.IsZero() || time.Since(lastProgressAt) >= stallTimeout {
			_ = tracer.LogWarn(tracer.ID_APP,
				"[FFMPEG] stalled target=%s no ffmpeg progress for %s, killing pid=%d",
				w.targetURL, stallTimeout, cmd.Process.Pid)
			_ = killProcess(cmd)
			return
		}
	}
}

func (w *ffmpegWorker) trackPtsFromFFmpegProgress(stderr io.ReadCloser, doneCh chan struct{}) {
	defer close(doneCh)
	defer stderr.Close()

	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if isFFmpegProgressLine(line) {
			w.lastProgressUnixMs.Store(time.Now().UnixMilli())
			if strings.HasPrefix(line, "out_time_us=") {
				raw := strings.TrimPrefix(line, "out_time_us=")
				if us, err := strconv.ParseInt(raw, 10, 64); err == nil && us >= 0 {
					w.lastPtsFnMs.Store(us / 1000)
				}
			} else if strings.HasPrefix(line, "out_time_ms=") {
				raw := strings.TrimPrefix(line, "out_time_ms=")
				if us, err := strconv.ParseInt(raw, 10, 64); err == nil && us >= 0 {
					w.lastPtsFnMs.Store(us / 1000)
				}
			}
			continue
		}
		_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG][%s] %s", w.targetURL, line)
	}
	if err := scanner.Err(); err != nil {
		_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] stderr scan failed target=%s: %v", w.targetURL, err)
	}
}

func isFFmpegProgressLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "frame="):
		return true
	case strings.HasPrefix(line, "fps="):
		return true
	case strings.HasPrefix(line, "stream_"):
		return true
	case strings.HasPrefix(line, "bitrate="):
		return true
	case strings.HasPrefix(line, "total_size="):
		return true
	case strings.HasPrefix(line, "out_time="):
		return true
	case strings.HasPrefix(line, "out_time_us="):
		return true
	case strings.HasPrefix(line, "out_time_ms="):
		return true
	case strings.HasPrefix(line, "dup_frames="):
		return true
	case strings.HasPrefix(line, "drop_frames="):
		return true
	case strings.HasPrefix(line, "speed="):
		return true
	case strings.HasPrefix(line, "progress="):
		return true
	default:
		return false
	}
}

func uniqueNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	ret := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		ret = append(ret, v)
	}
	return ret
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func killProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Kill(); err == nil {
		return nil
	}

	if runtime.GOOS == "windows" {
		taskkill := exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		if err := taskkill.Run(); err != nil {
			return err
		}
		return nil
	}

	return cmd.Process.Kill()
}

func buildFFmpegCommand(cfg FFmpegConfig) (*exec.Cmd, error) {
	var args []string

	isDShowInput := runtime.GOOS == "windows" && isDShowInputSpec(cfg.InputFile)
	encoderConfig := selectFFmpegEncoderConfig()

	args = append(args,
		"-hide_banner", "-loglevel", "info",
		"-progress", "pipe:2", "-stats_period", "0.5",
	)
	if isDShowInput {
		// DirectShow capture is already real-time; -re is for file inputs and can starve capture buffers.
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] dshow input uses live capture pacing without -re")
	} else {
		args = append(args, "-re", "-stream_loop", "-1")
	}

	if runtime.GOOS == "windows" && len(encoderConfig.HardwareArgs) > 0 {
		args = append(args, encoderConfig.HardwareArgs...)
	}

	inputArgs := []string{"-i", cfg.InputFile}
	effectiveInputFile := cfg.InputFile
	if runtime.GOOS == "windows" && isDShowInput {
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] dshow input detected on Windows, encoder=%s h264=%s hevc=%s", encoderConfig.Name, encoderConfig.H264Encoder, encoderConfig.HEVCEncoder)
		videoReq, audioReq := parseDShowInputSpec(cfg.InputFile)
		videoName, audioName := videoReq, audioReq
		videoDevices, audioDevices, probeErr := listDShowDevices()
		if probeErr != nil {
			_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] failed to probe dshow devices: %v", probeErr)
		} else {
			if len(videoDevices) > 0 {
				_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] dshow video devices: %s", strings.Join(videoDevices, " | "))
			}
			if len(audioDevices) > 0 {
				_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] dshow audio devices: %s", strings.Join(audioDevices, " | "))
			}

			videoName = resolveDShowDeviceName(videoReq, videoDevices)
			if audioReq != "" && len(audioDevices) == 0 {
				audioName = ""
				_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] requested dshow audio device %q but no audio devices were enumerated, falling back to video-only capture", audioReq)
			} else {
				audioName = resolveDShowDeviceName(audioReq, audioDevices)
			}

			if videoReq != "" && videoName != videoReq {
				_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] dshow video device remapped: %q -> %q", videoReq, videoName)
			}
			if audioReq != "" && audioName != audioReq {
				_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] dshow audio device remapped: %q -> %q", audioReq, audioName)
			}
		}

		inputSpec := buildDShowInputSpec(videoName, audioName)
		audioMap := ffmpegAudioMap(inputSpec)
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG][AudioDebug] dshow inputSpec=%q requestedAudio=%q resolvedAudio=%q audioMap=%s", inputSpec, audioReq, audioName, audioMap)
		probeDShowAudioDeviceOnce(audioName)
		inputArgs = []string{"-fflags", "+genpts", "-thread_queue_size", "1024", "-rtbufsize", "256M", "-use_wallclock_as_timestamps", "1", "-f", "dshow", "-i", inputSpec}
		effectiveInputFile = inputSpec
	} else if runtime.GOOS != "windows" {
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] running on Linux, utilizing libx264 (software)")
	}
	args = append(args, inputArgs...)

	if len(cfg.TargetURLs) == 0 {
		return nil, errors.New("target url is empty")
	}
	audioMap := ffmpegAudioMap(effectiveInputFile)
	if len(cfg.TargetURLs) >= 4 && len(cfg.Outputs) > 0 {
		logOutputAudioPlan(cfg.VideoLayout, cfg.TargetURLs, cfg.Outputs, audioMap)
		args = appendFourLevelOutputs(args, cfg.VideoLayout, cfg.TargetURLs, cfg.Outputs, encoderConfig, audioMap)
		cmd, err := newFFmpegCommand(args...)
		if err != nil {
			return nil, err
		}
		cmd.Stdout = io.Discard
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] command target=%s args=%s", strings.Join(cfg.TargetURLs, ","), strings.Join(args, " "))
		return cmd, nil
	}

	args = appendVideoRenditions(args, encoderConfig.H264Encoder, cfg.VideoLayout)
	args = append(args,
		"-map", audioMap,
	)
	args = appendAudioEncodingArgs(args, "aac", "128k")
	if len(cfg.TargetURLs) == 1 {
		args = appendRecoverableMpegTSOutput(args, cfg.TargetURLs[0])
	} else {
		teeTargets := make([]string, 0, len(cfg.TargetURLs))
		for _, targetURL := range cfg.TargetURLs {
			teeTargets = append(teeTargets, "[f=mpegts]"+targetURL)
		}
		args = append(args, "-f", "tee", strings.Join(teeTargets, "|"))
	}

	cmd, err := newFFmpegCommand(args...)
	if err != nil {
		return nil, err
	}
	cmd.Stdout = io.Discard
	_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] command target=%s args=%s", strings.Join(cfg.TargetURLs, ","), strings.Join(args, " "))
	return cmd, nil
}

func appendFourLevelOutputs(args []string, layout string, targets []string, outputs []OutputProfile, encoderConfig ffmpegEncoderConfig, audioMap string) []string {
	if encoderConfig.H264Encoder == "" {
		encoderConfig = defaultFFmpegEncoderConfig()
	}
	if strings.TrimSpace(audioMap) == "" {
		audioMap = "0:a:0?"
	}
	outputs = normalizeOutputProfiles(outputs, layout)
	for i, output := range outputs {
		if i >= len(targets) {
			break
		}
		audioCodec := strings.ToLower(strings.TrimSpace(output.AudioCodec))
		if audioCodec == "" {
			audioCodec = "aac"
		}
		audioBitrate := strings.TrimSpace(output.AudioBitrate)
		if audioBitrate == "" {
			audioBitrate = "128k"
		}
		if output.AudioOnly {
			args = append(args,
				"-map", audioMap,
				"-vn",
			)
			args = appendAudioEncodingArgs(args, audioCodec, audioBitrate)
			args = appendRecoverableMpegTSOutput(args, targets[i])
			continue
		}
		videoCodec := resolveVideoEncoder(output.VideoCodec, encoderConfig)
		videoArgs := []string{
			"-map", "0:v:0",
			"-map", audioMap,
			"-c:v", videoCodec,
			"-b:v", output.VideoBitrate,
			"-maxrate:v", output.VideoMaxrate,
		}
		if isH264Encoder(videoCodec) {
			videoArgs = append(videoArgs, "-profile:v", "high")
		}
		videoArgs = append(videoArgs,
			"-pix_fmt", "yuv420p",
			"-bf:v", "0",
			"-g:v", "30",
			"-vf", fmt.Sprintf("scale=%d:%d", selectedOutputWidth(output, layout), selectedOutputHeight(output, layout)),
		)
		videoArgs = appendAudioEncodingArgs(videoArgs, audioCodec, audioBitrate)
		videoArgs = appendRecoverableMpegTSOutput(videoArgs, targets[i])
		args = append(args, videoArgs...)
	}
	return args
}

func appendAudioEncodingArgs(args []string, codec, bitrate string) []string {
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec == "" {
		codec = "aac"
	}
	bitrate = strings.TrimSpace(bitrate)
	if bitrate == "" {
		bitrate = "128k"
	}
	args = append(args,
		"-c:a", codec,
		"-b:a", bitrate,
		"-ar", "48000",
		"-ac", "2",
	)
	switch codec {
	case "libopus", "opus":
		args = append(args,
			"-application:a", "audio",
			"-frame_duration:a", "20",
			"-vbr:a", "off",
			"-compression_level:a", "10",
		)
	}
	return args
}

func ffmpegAudioMap(inputFile string) string {
	_, audioName := parseDShowInputSpec(inputFile)
	if strings.TrimSpace(audioName) != "" {
		return "0:a:0"
	}
	if probeFileAudioOnce(inputFile) {
		return "0:a:0"
	}
	return "0:a:0?"
}

func logOutputAudioPlan(layout string, targets []string, outputs []OutputProfile, audioMap string) {
	outputs = normalizeOutputProfiles(outputs, layout)
	parts := make([]string, 0, len(outputs))
	for i, output := range outputs {
		if i >= len(targets) {
			break
		}
		codec := strings.ToLower(strings.TrimSpace(output.AudioCodec))
		if codec == "" {
			codec = "aac"
		}
		bitrate := strings.TrimSpace(output.AudioBitrate)
		if bitrate == "" {
			bitrate = "128k"
		}
		parts = append(parts, fmt.Sprintf("%s:%s@%s", output.Level, codec, bitrate))
	}
	_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG][AudioDebug] output audioMap=%s outputs=%s", audioMap, strings.Join(parts, " | "))
}

func probeDShowAudioDeviceOnce(audioName string) {
	audioName = strings.TrimSpace(audioName)
	if audioName == "" {
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG][AudioProbe] skipped: no audio device selected")
		return
	}
	if _, loaded := dshowAudioProbeCache.LoadOrStore(audioName, struct{}{}); loaded {
		return
	}
	output, err := probeDShowAudioDevice(audioName)
	if err != nil {
		_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG][AudioProbe] failed audio=%q err=%v output=%s", audioName, err, compactLogText(output, 2000))
		return
	}
	_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG][AudioProbe] ok audio=%q output=%s", audioName, compactLogText(output, 2000))
}

func probeDShowAudioDevice(audioName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exe, err := ffmpegExecutable()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, exe,
		"-hide_banner",
		"-loglevel", "info",
		"-f", "dshow",
		"-i", "audio="+audioName,
		"-t", "1",
		"-vn",
		"-af", "volumedetect",
		"-f", "null",
		os.DevNull,
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(output), ctx.Err()
	}
	return string(output), err
}

func probeFileAudioOnce(inputFile string) bool {
	inputFile = strings.TrimSpace(inputFile)
	if inputFile == "" || isDShowInputSpec(inputFile) {
		return false
	}
	if info, err := os.Stat(inputFile); err != nil || info.IsDir() {
		return false
	}
	if v, ok := fileAudioProbeCache.Load(inputFile); ok {
		return v.(fileAudioProbeResult).hasAudio
	}
	output, err := probeFileAudio(inputFile)
	result := fileAudioProbeResult{hasAudio: err == nil}
	fileAudioProbeCache.Store(inputFile, result)
	if err != nil {
		_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG][AudioProbe] file failed input=%q err=%v output=%s", inputFile, err, compactLogText(output, 2000))
		return false
	}
	_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG][AudioProbe] file ok input=%q output=%s", inputFile, compactLogText(output, 2000))
	return true
}

func probeFileAudio(inputFile string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exe, err := ffmpegExecutable()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, exe,
		"-hide_banner",
		"-loglevel", "info",
		"-i", inputFile,
		"-t", "1",
		"-map", "0:a:0",
		"-vn",
		"-af", "volumedetect",
		"-f", "null",
		os.DevNull,
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(output), ctx.Err()
	}
	return string(output), err
}

func appendRecoverableMpegTSOutput(args []string, target string) []string {
	return append(args,
		"-f", "fifo",
		"-fifo_format", "mpegts",
		"-format_opts", "mpegts_flags=+resend_headers+system_b",
		"-queue_size", "180",
		"-drop_pkts_on_overflow", "1",
		"-attempt_recovery", "1",
		"-recover_any_error", "1",
		"-recovery_wait_time", "1",
		"-max_recovery_attempts", "1000000",
		target,
	)
}

func resolveVideoEncoder(videoCodec string, encoderConfig ffmpegEncoderConfig) string {
	if encoderConfig.H264Encoder == "" {
		encoderConfig = defaultFFmpegEncoderConfig()
	}
	switch strings.ToLower(strings.TrimSpace(videoCodec)) {
	case "h264", "libx264", "h264_qsv", "h264_nvenc":
		return encoderConfig.H264Encoder
	case "hevc", "h265", "libx265", "hevc_qsv", "hevc_nvenc":
		if strings.TrimSpace(encoderConfig.HEVCEncoder) != "" {
			return encoderConfig.HEVCEncoder
		}
		_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] HEVC encoder unavailable for mode=%s, falling back to %s", encoderConfig.Name, encoderConfig.H264Encoder)
		return encoderConfig.H264Encoder
	default:
		return encoderConfig.H264Encoder
	}
}

func defaultFFmpegEncoderConfig() ffmpegEncoderConfig {
	if runtime.GOOS == "windows" {
		return ffmpegEncoderConfig{
			Name:         "qsv",
			H264Encoder:  "h264_qsv",
			HEVCEncoder:  "hevc_qsv",
			HardwareArgs: []string{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"},
		}
	}
	return ffmpegEncoderConfig{Name: "software", H264Encoder: "libx264", HEVCEncoder: "libx265"}
}

func selectFFmpegEncoderConfig() ffmpegEncoderConfig {
	if runtime.GOOS != "windows" {
		return defaultFFmpegEncoderConfig()
	}
	windowsEncoderOnce.Do(func() {
		windowsEncoderConfig = detectWindowsEncoderConfig()
	})
	return windowsEncoderConfig
}

func detectWindowsEncoderConfig() ffmpegEncoderConfig {
	if out, err := runFFmpegProbe(
		"-hide_banner", "-loglevel", "error",
		"-init_hw_device", "qsv=hw",
		"-f", "lavfi", "-i", "testsrc2=size=64x64:rate=1:duration=0.1",
		"-frames:v", "1",
		"-an",
		"-c:v", "h264_qsv",
		"-f", "null", "-",
	); err == nil {
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] encoder probe selected Intel QSV")
		return defaultFFmpegEncoderConfig()
	} else {
		_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] Intel QSV unavailable, probe err=%v output=%s", err, compactLogText(out, 2000))
	}

	if out, err := runFFmpegProbe(
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=64x64:rate=1:duration=0.1",
		"-frames:v", "1",
		"-an",
		"-c:v", "h264_nvenc",
		"-f", "null", "-",
	); err == nil {
		hevcEncoder := "hevc_nvenc"
		if hevcOut, hevcErr := runFFmpegProbe(
			"-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "testsrc2=size=64x64:rate=1:duration=0.1",
			"-frames:v", "1",
			"-an",
			"-c:v", "hevc_nvenc",
			"-f", "null", "-",
		); hevcErr != nil {
			hevcEncoder = ""
			_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] NVIDIA HEVC NVENC unavailable, probe err=%v output=%s", hevcErr, compactLogText(hevcOut, 2000))
		}
		_ = tracer.LogInfo(tracer.ID_APP, "[FFMPEG] encoder probe selected NVIDIA NVENC hevc=%s", hevcEncoder)
		return ffmpegEncoderConfig{Name: "nvenc", H264Encoder: "h264_nvenc", HEVCEncoder: hevcEncoder}
	} else {
		_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] NVIDIA NVENC unavailable, probe err=%v output=%s", err, compactLogText(out, 2000))
	}

	_ = tracer.LogWarn(tracer.ID_APP, "[FFMPEG] no hardware encoder available, falling back to software encoders")
	return ffmpegEncoderConfig{Name: "software", H264Encoder: "libx264", HEVCEncoder: "libx265"}
}

func configuredFFmpegEncoderConfig(name string) ffmpegEncoderConfig {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "qsv":
		return ffmpegEncoderConfig{
			Name:         "qsv",
			H264Encoder:  "h264_qsv",
			HEVCEncoder:  "hevc_qsv",
			HardwareArgs: []string{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"},
		}
	case "nvenc":
		return ffmpegEncoderConfig{Name: "nvenc", H264Encoder: "h264_nvenc", HEVCEncoder: "hevc_nvenc"}
	case "software":
		return ffmpegEncoderConfig{Name: "software", H264Encoder: "libx264", HEVCEncoder: "libx265"}
	default:
		return defaultFFmpegEncoderConfig()
	}
}

func isH264Encoder(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "libx264", "h264_qsv", "h264_nvenc":
		return true
	default:
		return false
	}
}

func appendVideoRenditions(args []string, codec, layout string) []string {
	renditions := buildVideoRenditions(layout)
	for i, r := range renditions {
		args = append(args,
			"-map", "0:v:0", fmt.Sprintf("-c:v:%d", i), codec,
			fmt.Sprintf("-b:v:%d", i), r.bitrate, fmt.Sprintf("-maxrate:v:%d", i), r.maxrate,
		)
		if isH264Encoder(codec) {
			args = append(args, fmt.Sprintf("-profile:v:%d", i), "high")
		}
		args = append(args,
			fmt.Sprintf("-pix_fmt:v:%d", i), "yuv420p",
			fmt.Sprintf("-bf:v:%d", i), "0", fmt.Sprintf("-g:v:%d", i), "30",
			fmt.Sprintf("-filter:v:%d", i), fmt.Sprintf("scale=%d:%d", r.width, r.height),
		)
	}
	return args
}

type videoRendition struct {
	width   int
	height  int
	bitrate string
	maxrate string
}

func buildVideoRenditions(layout string) []videoRendition {
	layout = strings.ToLower(strings.TrimSpace(layout))
	portrait := []videoRendition{
		{width: 360, height: 640, bitrate: "400k", maxrate: "600k"},
		{width: 720, height: 1280, bitrate: "1000k", maxrate: "1500k"},
		{width: 1080, height: 1920, bitrate: "2000k", maxrate: "3000k"},
	}
	landscape := []videoRendition{
		{width: 640, height: 360, bitrate: "400k", maxrate: "600k"},
		{width: 1280, height: 720, bitrate: "1000k", maxrate: "1500k"},
		{width: 1920, height: 1080, bitrate: "2000k", maxrate: "3000k"},
	}

	switch layout {
	case "landscape":
		return landscape
	case "both":
		out := make([]videoRendition, 0, len(portrait)+len(landscape))
		out = append(out, portrait...)
		out = append(out, landscape...)
		return out
	default:
		return portrait
	}
}

func normalizeOutputProfiles(outputs []OutputProfile, layout string) []OutputProfile {
	if len(outputs) == 0 {
		renditions := buildVideoRenditions(layout)
		return []OutputProfile{
			{Level: "bottom", AudioOnly: true, AudioCodec: "aac", AudioBitrate: "128k"},
			{Level: "economic", VideoCodec: "h264", VideoBitrate: renditions[0].bitrate, VideoMaxrate: renditions[0].maxrate, PortraitWidth: 360, PortraitHeight: 640, LandscapeWidth: 640, LandscapeHeight: 360, AudioCodec: "aac", AudioBitrate: "128k"},
			{Level: "standard_hevc", VideoCodec: "hevc", VideoBitrate: "600k", VideoMaxrate: "1000k", PortraitWidth: 720, PortraitHeight: 1280, LandscapeWidth: 1280, LandscapeHeight: 720, AudioCodec: "aac", AudioBitrate: "128k"},
			{Level: "standard", VideoCodec: "h264", VideoBitrate: renditions[1].bitrate, VideoMaxrate: renditions[1].maxrate, PortraitWidth: 720, PortraitHeight: 1280, LandscapeWidth: 1280, LandscapeHeight: 720, AudioCodec: "aac", AudioBitrate: "128k"},
			{Level: "high", VideoCodec: "h264", VideoBitrate: renditions[2].bitrate, VideoMaxrate: renditions[2].maxrate, PortraitWidth: 1080, PortraitHeight: 1920, LandscapeWidth: 1920, LandscapeHeight: 1080, AudioCodec: "aac", AudioBitrate: "128k"},
		}
	}
	out := make([]OutputProfile, len(outputs))
	copy(out, outputs)
	defaults := normalizeOutputProfiles(nil, layout)
	defaultByLevel := make(map[string]OutputProfile, len(defaults))
	for _, def := range defaults {
		defaultByLevel[strings.ToLower(strings.TrimSpace(def.Level))] = def
	}
	for i := range out {
		level := strings.ToLower(strings.TrimSpace(out[i].Level))
		def, ok := defaultByLevel[level]
		if !ok {
			if i >= len(defaults) {
				continue
			}
			def = defaults[i]
		}
		if strings.TrimSpace(out[i].Level) == "" {
			out[i].Level = def.Level
		}
		if strings.TrimSpace(out[i].VideoCodec) == "" {
			out[i].VideoCodec = def.VideoCodec
		}
		if strings.TrimSpace(out[i].VideoBitrate) == "" {
			out[i].VideoBitrate = def.VideoBitrate
		}
		if strings.TrimSpace(out[i].VideoMaxrate) == "" {
			out[i].VideoMaxrate = def.VideoMaxrate
		}
		if out[i].PortraitWidth <= 0 {
			out[i].PortraitWidth = def.PortraitWidth
		}
		if out[i].PortraitHeight <= 0 {
			out[i].PortraitHeight = def.PortraitHeight
		}
		if out[i].LandscapeWidth <= 0 {
			out[i].LandscapeWidth = def.LandscapeWidth
		}
		if out[i].LandscapeHeight <= 0 {
			out[i].LandscapeHeight = def.LandscapeHeight
		}
		if strings.TrimSpace(out[i].AudioCodec) == "" {
			out[i].AudioCodec = def.AudioCodec
		}
		if strings.TrimSpace(out[i].AudioBitrate) == "" {
			out[i].AudioBitrate = def.AudioBitrate
		}
		if def.AudioOnly {
			out[i].AudioOnly = true
		}
	}
	return out
}

func selectedOutputWidth(output OutputProfile, layout string) int {
	if strings.EqualFold(strings.TrimSpace(layout), "landscape") {
		return output.LandscapeWidth
	}
	return output.PortraitWidth
}

func selectedOutputHeight(output OutputProfile, layout string) int {
	if strings.EqualFold(strings.TrimSpace(layout), "landscape") {
		return output.LandscapeHeight
	}
	return output.PortraitHeight
}

func parseDShowInputSpec(input string) (string, string) {
	var videoName string
	var audioName string

	parts := strings.Split(input, ":")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		if strings.HasPrefix(lower, "video=") {
			videoName = trimDShowDeviceValue(part[len("video="):])
			continue
		}
		if strings.HasPrefix(lower, "audio=") {
			audioName = trimDShowDeviceValue(part[len("audio="):])
		}
	}
	return videoName, audioName
}

func trimDShowDeviceValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"") {
		v = v[1 : len(v)-1]
	}
	return strings.TrimSpace(v)
}

func buildDShowInputSpec(videoName, audioName string) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(videoName) != "" {
		parts = append(parts, "video="+strings.TrimSpace(videoName))
	}
	if strings.TrimSpace(audioName) != "" {
		parts = append(parts, "audio="+strings.TrimSpace(audioName))
	}
	return strings.Join(parts, ":")
}

func listDShowDevices() ([]string, []string, error) {
	cmd, err := newFFmpegCommand("-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
	if err != nil {
		return nil, nil, err
	}
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	err = cmd.Run()

	rawOutput := stderr.String()
	_ = tracer.LogInfo(tracer.ID_APP, "[DeviceProbe] dshow list command err=%v raw=%s", err, compactLogText(rawOutput, 12000))
	videoDevices, audioDevices := parseDShowDevices(rawOutput)
	if len(videoDevices) == 0 && len(audioDevices) == 0 && err != nil {
		return nil, nil, err
	}
	return videoDevices, audioDevices, nil
}

func logFFmpegDeviceProbeContext() {
	if path, err := ffmpegExecutable(); err != nil {
		_ = tracer.LogWarn(tracer.ID_APP, "[DeviceProbe] ffmpeg lookup failed: %v", err)
	} else {
		_ = tracer.LogInfo(tracer.ID_APP, "[DeviceProbe] ffmpeg path=%s", path)
	}

	if out, err := runFFmpegProbe("-hide_banner", "-version"); err != nil {
		_ = tracer.LogWarn(tracer.ID_APP, "[DeviceProbe] ffmpeg version failed: %v output=%s", err, compactLogText(out, 2000))
	} else {
		_ = tracer.LogInfo(tracer.ID_APP, "[DeviceProbe] ffmpeg version=%s", firstNonEmptyLine(out))
	}

	if out, err := runFFmpegProbe("-hide_banner", "-devices"); err != nil {
		_ = tracer.LogWarn(tracer.ID_APP, "[DeviceProbe] ffmpeg devices failed: %v output=%s", err, compactLogText(out, 4000))
	} else {
		_ = tracer.LogInfo(tracer.ID_APP, "[DeviceProbe] ffmpeg devices=%s", compactLogText(out, 4000))
	}

	if out, err := runFFmpegProbe("-hide_banner", "-sources", "dshow"); err != nil {
		_ = tracer.LogWarn(tracer.ID_APP, "[DeviceProbe] ffmpeg sources dshow failed: %v output=%s", err, compactLogText(out, 12000))
	} else {
		_ = tracer.LogInfo(tracer.ID_APP, "[DeviceProbe] ffmpeg sources dshow=%s", compactLogText(out, 12000))
	}
}

func runFFmpegProbe(args ...string) (string, error) {
	cmd, err := newFFmpegCommand(args...)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	return out.String(), err
}

func newFFmpegCommand(args ...string) (*exec.Cmd, error) {
	exe, err := ffmpegExecutable()
	if err != nil {
		return nil, err
	}
	return exec.Command(exe, args...), nil
}

func ffmpegExecutable() (string, error) {
	candidates := make([]string, 0, 5)
	if envPath := strings.TrimSpace(os.Getenv("FFMPEG_PATH")); envPath != "" {
		candidates = append(candidates, envPath)
	}
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), ffmpegBinaryName()))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, ffmpegBinaryName()),
			filepath.Join(cwd, "bin", ffmpegBinaryName()),
		)
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs, nil
			}
			return candidate, nil
		}
	}

	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		if errors.Is(err, exec.ErrDot) && path != "" {
			return filepath.Abs(path)
		}
		return "", err
	}
	return path, nil
}

func ffmpegBinaryName() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}

func compactLogText(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	text = strings.ReplaceAll(text, "\n", " || ")
	if limit > 0 && len(text) > limit {
		return text[:limit] + "...<truncated>"
	}
	return text
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func parseDShowDevices(output string) ([]string, []string) {
	videoDevices := make([]string, 0)
	audioDevices := make([]string, 0)
	seenVideo := map[string]struct{}{}
	seenAudio := map[string]struct{}{}

	section := ""
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "directshow video devices"):
			section = "video"
			continue
		case strings.Contains(lower, "directshow audio devices"):
			section = "audio"
			continue
		case strings.Contains(lower, "\"") && strings.Contains(lower, "(video)"):
			section = "video"
		case strings.Contains(lower, "\"") && strings.Contains(lower, "(audio)"):
			section = "audio"
		}

		first := strings.Index(line, "\"")
		if first < 0 {
			continue
		}
		rest := line[first+1:]
		second := strings.Index(rest, "\"")
		if second < 0 {
			continue
		}
		name := strings.TrimSpace(rest[:second])
		if name == "" {
			continue
		}

		switch section {
		case "video":
			if _, ok := seenVideo[name]; ok {
				continue
			}
			seenVideo[name] = struct{}{}
			videoDevices = append(videoDevices, name)
		case "audio":
			if _, ok := seenAudio[name]; ok {
				continue
			}
			seenAudio[name] = struct{}{}
			audioDevices = append(audioDevices, name)
		}
	}

	return videoDevices, audioDevices
}

func resolveDShowDeviceName(requested string, devices []string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return ""
	}
	if len(devices) == 0 {
		return requested
	}

	reqLower := strings.ToLower(requested)
	for _, dev := range devices {
		if strings.EqualFold(strings.TrimSpace(dev), requested) {
			return dev
		}
	}

	matches := make([]string, 0, 2)
	for _, dev := range devices {
		devLower := strings.ToLower(strings.TrimSpace(dev))
		if strings.Contains(devLower, reqLower) || strings.Contains(reqLower, devLower) {
			matches = append(matches, dev)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}

	return requested
}

func isDShowInputSpec(input string) bool {
	v := strings.TrimSpace(strings.ToLower(input))
	return strings.HasPrefix(v, "video=") || strings.HasPrefix(v, "audio=")
}

func shouldUseSharedFFmpeg(inputFile string, targets []string) bool {
	if len(targets) <= 1 {
		return false
	}
	if len(targets) >= 4 {
		return true
	}
	// Live capture input (dshow) can usually be opened by only one ffmpeg process.
	return runtime.GOOS == "windows" && isDShowInputSpec(inputFile)
}

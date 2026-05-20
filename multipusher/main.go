package main

import (
	"fmt"
	"image/color"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"vpublisher/conf"
	"vpublisher/tracer"
	"vpublisher/utils"
	"vpublisher/ws"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	Version   = "v1.0.6"
	BuildTime = "unknown"
	GitCommit = "unknown"
	Author    = "Amor"
)

const AppVersion = "v1.0.6"

const playStatURL = "https://videostat-test.example.com/dashboard/"
const noAudioOption = "(No audio)"

var siteNameOptions = []string{"studio_3drush", "studio_gsp2w"}

var streamNamesBySiteName = map[string][]string{
	"studio_3drush": {"3drush-fwh", "3drush-fwv"},
	"studio_gsp2w":  {"gsp2w-fwv", "gsp2w-fwh"},
}

var siteNameByStreamName = map[string]string{
	"3drush-fwh": "studio_3drush",
	"3drush-fwv": "studio_3drush",
	"gsp2w-fwv":  "studio_gsp2w",
	"gsp2w-fwh":  "studio_gsp2w",
}

var (
	statusIdleColor     = color.NRGBA{R: 120, G: 120, B: 120, A: 255}
	statusStartingColor = color.NRGBA{R: 190, G: 135, B: 25, A: 255}
	statusRunningColor  = color.NRGBA{R: 30, G: 145, B: 75, A: 255}
	statusStoppedColor  = color.NRGBA{R: 195, G: 65, B: 65, A: 255}
)

type streamPreview struct {
	Level             string
	StreamName        string
	EncodingParameter string
	URL               string
	Status            string
}

type refreshOnOpenSelect struct {
	widget.Select
	onOpen func()
}

func newRefreshOnOpenSelect(options []string, changed func(string), onOpen func()) *refreshOnOpenSelect {
	s := &refreshOnOpenSelect{onOpen: onOpen}
	s.Options = options
	s.OnChanged = changed
	s.PlaceHolder = "(Select one)"
	s.ExtendBaseWidget(s)
	return s
}

func (s *refreshOnOpenSelect) Tapped(ev *fyne.PointEvent) {
	if s.onOpen != nil && !s.Disabled() {
		s.onOpen()
	}
	s.Select.Tapped(ev)
}

func main() {
	tracer.InitLog(tracer.DEBUG, "multipusher-ui")
	_ = tracer.LogInfo(tracer.ID_APP, "starting multipusher ui version=%s buildTime=%s commit=%s author=%s", Version, BuildTime, GitCommit, Author)

	a := app.NewWithID("tencent-abrplayer.multipusher")
	w := a.NewWindow(appTitle())
	w.Resize(fyne.NewSize(1080, 760))

	player := newPlayerClient()
	_ = tracer.LogInfo(tracer.ID_APP, "abrplayer backend URL=%s", player.baseURL)

	ui := newMainUI(w, player)
	w.SetContent(ui.root)
	a.Lifecycle().SetOnStarted(func() {
		ui.startOnReady()
	})
	w.SetCloseIntercept(func() {
		if ui.running.Load() {
			dialog.ShowConfirm("Stop publisher", "Publisher is running. Stop it and close?", func(ok bool) {
				if !ok {
					return
				}
				_ = ws.StopFFmpegPublisher()
				ui.close()
				w.Close()
			}, w)
			return
		}
		ui.close()
		w.Close()
	})
	w.ShowAndRun()
}

type mainUI struct {
	window fyne.Window
	root   fyne.CanvasObject

	configPath string
	running    atomic.Bool
	player     *playerClient
	statusStop chan struct{}
	uiMu       sync.Mutex

	siteName   *widget.Select
	streamName *widget.Select
	srtHost    *widget.Entry
	srtPort    *widget.Entry
	srtApp     *widget.Entry
	tokenDays  *widget.Entry
	inputMode  *widget.RadioGroup
	videoInput *refreshOnOpenSelect
	audioInput *refreshOnOpenSelect
	videoFile  *widget.Entry
	layout     *widget.RadioGroup
	onReady    *widget.Check

	formBox *fyne.Container
	preview *widget.Table
	logs    *widget.Entry

	startButton *widget.Button
	stopButton  *widget.Button

	previews     []streamPreview
	targetStatus map[string]string
	streams      []conf.StreamConfig
	secrets      conf.SecretsConfig
}

func newMainUI(w fyne.Window, player *playerClient) *mainUI {
	ui := &mainUI{window: w, configPath: defaultConfigPath(), player: player, targetStatus: make(map[string]string), streams: conf.DefaultStreams()}
	ui.siteName = widget.NewSelect(siteNameOptions, func(siteName string) {
		ui.syncStreamNameWithSite(siteName)
		ui.refreshPreview()
	})
	ui.streamName = widget.NewSelect(nil, func(streamName string) {
		ui.applyLayoutForStreamName(streamName)
		ui.refreshPreview()
	})
	ui.srtHost = widget.NewEntry()
	ui.srtPort = widget.NewEntry()
	ui.srtApp = widget.NewEntry()
	ui.tokenDays = widget.NewEntry()
	ui.inputMode = widget.NewRadioGroup([]string{"device", "file"}, func(string) {
		ui.updateInputModeFields()
		ui.refreshPreview()
		ui.noteRuntimeConfigChange("input mode")
	})
	ui.inputMode.Horizontal = true
	ui.videoInput = newRefreshOnOpenSelect(nil, func(string) {
		ui.refreshPreview()
		ui.noteRuntimeConfigChange("video input")
	}, func() {
		ui.refreshDevices()
	})
	ui.audioInput = newRefreshOnOpenSelect(nil, func(string) {
		ui.refreshPreview()
		ui.noteRuntimeConfigChange("audio input")
	}, func() {
		ui.refreshDevices()
	})
	ui.audioInput.Options = []string{noAudioOption}
	ui.audioInput.SetSelected(noAudioOption)
	ui.videoFile = widget.NewEntry()
	ui.videoFile.SetPlaceHolder("Select an .mp4 file for test input")
	ui.videoFile.OnChanged = func(string) {
		ui.refreshPreview()
		ui.noteRuntimeConfigChange("video file")
	}
	ui.layout = widget.NewRadioGroup([]string{"portrait", "landscape"}, func(string) {
		ui.refreshPreview()
		ui.noteRuntimeConfigChange("layout")
	})
	ui.layout.Horizontal = true
	ui.onReady = widget.NewCheck("Publish on UI start", nil)
	ui.logs = widget.NewMultiLineEntry()
	ui.logs.SetMinRowsVisible(14)
	ui.logs.Wrapping = fyne.TextWrapWord

	ui.preview = widget.NewTable(
		func() (int, int) { return len(ui.previews) + 1, 4 },
		func() fyne.CanvasObject { return canvas.NewText("", theme.ForegroundColor()) },
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			label := obj.(*canvas.Text)
			label.Color = theme.ForegroundColor()
			if id.Row == 0 {
				switch id.Col {
				case 0:
					label.Text = "Level"
				case 1:
					label.Text = "Stream"
				case 2:
					label.Text = "Encoding parameter"
				case 3:
					label.Text = "Status"
				}
				label.TextStyle = fyne.TextStyle{Bold: true}
				label.Refresh()
				return
			}
			label.TextStyle = fyne.TextStyle{}
			row := ui.previews[id.Row-1]
			switch id.Col {
			case 0:
				label.Text = row.Level
			case 1:
				label.Text = row.StreamName
			case 2:
				label.Text = row.EncodingParameter
			case 3:
				label.Text = row.Status
				label.Color = statusColor(row.Status)
			}
			label.Refresh()
		},
	)
	ui.preview.SetColumnWidth(0, 100)
	ui.preview.SetColumnWidth(1, 210)
	ui.preview.SetColumnWidth(2, 520)
	ui.preview.SetColumnWidth(3, 120)

	ui.loadConfigIfExists()
	ui.refreshPreview()

	ui.formBox = container.NewVBox()
	ui.updateInputModeFields()

	for _, entry := range []*widget.Entry{ui.srtHost, ui.srtPort, ui.srtApp, ui.tokenDays} {
		entry.OnChanged = func(string) {
			ui.refreshPreview()
			ui.noteRuntimeConfigChange("publish settings")
		}
	}

	ui.startButton = widget.NewButton("start-pub", ui.startAction)
	ui.startButton.Importance = widget.SuccessImportance
	ui.stopButton = widget.NewButton("stop-pub", ui.stopAction)
	ui.stopButton.Importance = widget.DangerImportance
	ui.updatePublisherButtons()
	playButton := widget.NewButton("go-play", ui.playAction)
	playButton.Importance = widget.HighImportance
	playStatButton := widget.NewButton("play-stat", ui.playStatAction)
	playStatButton.Importance = widget.WarningImportance
	buttons := container.NewHBox(
		ui.startButton,
		fixedSpacer(18),
		ui.stopButton,
		fixedSpacer(18),
		playButton,
		fixedSpacer(18),
		playStatButton,
	)

	top := container.NewVBox(
		ui.formBox,
		buttons,
		widget.NewLabelWithStyle("Five-stream output", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)
	mainContent := container.NewBorder(top, nil, nil, nil, ui.preview)
	bottom := container.NewBorder(
		widget.NewLabelWithStyle("Logs", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		ui.logs,
	)
	ui.root = container.NewBorder(nil, bottom, nil, nil, mainContent)
	ui.appendLog("config path: " + ui.configPath)
	if ui.inputMode.Selected == "device" {
		ui.refreshDevices()
	}
	return ui
}

func (ui *mainUI) startOnReady() {
	if ui == nil || ui.onReady == nil || !ui.onReady.Checked {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			ui.running.Store(false)
			ui.updatePublisherButtons()
			ui.setAllPreviewStatus("Stopped")
			ui.appendLog(fmt.Sprintf("publisher auto-start failed; abrplayer backend is separate: %v", r))
			_ = tracer.LogError(tracer.ID_APP, "publisher auto-start panic: %v", r)
		}
	}()
	ui.appendLog("publish.onReady=true, starting publisher")
	if err := ui.startPublisher(false); err != nil {
		ui.appendLog("publisher auto-start failed; abrplayer backend is separate: " + err.Error())
		_ = tracer.LogWarn(tracer.ID_APP, "publisher auto-start failed: %v", err)
	}
}

func appTitle() string {
	return fmt.Sprintf("Multipusher %s %s@2026", AppVersion, Author)
}

func fixedSpacer(width float32) fyne.CanvasObject {
	spacer := canvas.NewRectangle(color.NRGBA{})
	spacer.SetMinSize(fyne.NewSize(width, 1))
	return spacer
}

func (ui *mainUI) close() {
	ui.stopStatusPolling()
	if ui.player != nil {
		ui.player.Close()
		ui.player = nil
	}
}

func (ui *mainUI) buildFormItems() []*widget.FormItem {
	siteStreamRow := container.NewGridWithColumns(2,
		container.NewBorder(nil, nil, widget.NewLabel("Site name"), nil, ui.siteName),
		container.NewBorder(nil, nil, widget.NewLabel("Stream name"), nil, ui.streamName),
	)
	items := []*widget.FormItem{
		widget.NewFormItem("", siteStreamRow),
		widget.NewFormItem("Input mode", ui.inputMode),
	}
	if ui.inputMode.Selected == "file" {
		items = append(items, widget.NewFormItem("Video file", container.NewBorder(nil, nil, nil, widget.NewButton("Browse", ui.chooseVideoFile), ui.videoFile)))
	} else {
		items = append(items,
			widget.NewFormItem("Video input", ui.videoInput),
			widget.NewFormItem("Audio input", ui.audioInput),
		)
	}
	items = append(items, widget.NewFormItem("Layout", ui.layout))
	return items
}

func (ui *mainUI) updateInputModeFields() {
	if ui.formBox == nil {
		return
	}
	ui.formBox.Objects = []fyne.CanvasObject{widget.NewForm(ui.buildFormItems()...)}
	ui.formBox.Refresh()
}

func (ui *mainUI) syncStreamNameWithSite(siteName string) {
	siteName = strings.TrimSpace(siteName)
	ui.setStreamNameForSite(siteName, defaultStreamNameForSite(siteName))
}

func (ui *mainUI) setStreamNameForSite(siteName, selectedStreamName string) {
	siteName = strings.TrimSpace(siteName)
	selectedStreamName = strings.TrimSpace(selectedStreamName)
	options := append([]string(nil), streamNamesBySiteName[siteName]...)
	defaultStreamName := defaultStreamNameForSite(siteName)
	if selectedStreamName == "" {
		selectedStreamName = defaultStreamName
	}
	ui.streamName.Options = uniqueSelectOptions(options, selectedStreamName)
	ui.streamName.Refresh()
	ui.streamName.SetSelected(selectedStreamName)
}

func (ui *mainUI) selectedStreamName() string {
	if ui.streamName == nil {
		return defaultStreamNameForSite("")
	}
	streamName := strings.TrimSpace(ui.streamName.Selected)
	if streamName != "" {
		return streamName
	}
	if ui.siteName == nil {
		return defaultStreamNameForSite("")
	}
	return defaultStreamNameForSite(strings.TrimSpace(ui.siteName.Selected))
}

func (ui *mainUI) selectedVideoLayout() string {
	if layout := layoutForStreamName(ui.selectedStreamName()); layout != "" {
		return layout
	}
	if ui.layout == nil {
		return "portrait"
	}
	return strings.TrimSpace(ui.layout.Selected)
}

func (ui *mainUI) applyLayoutForStreamName(streamName string) {
	layout := layoutForStreamName(streamName)
	if layout == "" || ui.layout == nil || ui.layout.Selected == layout {
		return
	}
	ui.layout.SetSelected(layout)
}

func (ui *mainUI) chooseVideoFile() {
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()
		if reader.URI() == nil {
			return
		}
		path := reader.URI().Path()
		if !strings.EqualFold(filepath.Ext(path), ".mp4") {
			ui.showError(fmt.Errorf("video file must be .mp4"))
			return
		}
		ui.videoFile.SetText(path)
		ui.inputMode.SetSelected("file")
	}, ui.window)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".mp4"}))
	fd.Show()
}

func defaultConfigPath() string {
	candidates := []string{
		// filepath.Join("bin", "conf", "pusher.local.yml"),
		filepath.Join("conf", "pusher.local.yml"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return candidates[0]
}

func (ui *mainUI) loadConfigIfExists() {
	cfg, err := conf.Load(ui.configPath)
	if err != nil {
		ui.applyConfig(defaultConfig())
		ui.appendLog(fmt.Sprintf("load config skipped: %v", err))
		return
	}
	ui.applyConfig(cfg)
}

func (ui *mainUI) loadConfigDialog() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()
		if reader.URI() != nil {
			ui.configPath = reader.URI().Path()
		}
		cfg, err := conf.Load(ui.configPath)
		if err != nil {
			ui.showError(err)
			return
		}
		ui.applyConfig(cfg)
		ui.refreshPreview()
		ui.appendLog("loaded config: " + ui.configPath)
	}, ui.window)
}

func (ui *mainUI) saveConfigAction() {
	cfg, err := ui.readConfig()
	if err != nil {
		ui.showError(err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(ui.configPath), 0750); err != nil {
		ui.showError(err)
		return
	}
	if err := conf.Save(ui.configPath, cfg); err != nil {
		ui.showError(err)
		return
	}
	ui.appendLog("saved config: " + ui.configPath)
}

func (ui *mainUI) refreshDevices() {
	ui.appendLog("probing input devices...")
	_ = tracer.LogInfo(tracer.ID_APP, "[DeviceProbeUI] refresh start selectedVideo=%q selectedAudio=%q",
		ui.videoInput.Selected, ui.audioInput.Selected)
	devices, err := ws.ListInputDevices()
	if err != nil {
		ui.appendLog("device probe warning: " + err.Error())
		_ = tracer.LogWarn(tracer.ID_APP, "[DeviceProbeUI] refresh warning: %v", err)
	}
	if len(devices.Video) > 0 {
		ui.videoInput.Options = uniqueSelectOptions(devices.Video, ui.videoInput.Selected)
		ui.videoInput.Refresh()
		if ui.videoInput.Selected == "" {
			ui.videoInput.SetSelected(devices.Video[0])
		}
	}
	audioSelected := audioDeviceValue(ui.audioInput.Selected)
	audioOptions := append([]string{noAudioOption}, devices.Audio...)
	ui.audioInput.Options = uniqueSelectOptions(audioOptions, audioSelected)
	ui.audioInput.Refresh()
	if audioSelected == "" {
		ui.audioInput.SetSelected(noAudioOption)
	} else {
		ui.audioInput.SetSelected(audioSelected)
	}
	_ = tracer.LogInfo(tracer.ID_APP, "[DeviceProbeUI] refresh result video=%d audio=%d selectedVideo=%q selectedAudio=%q videoOptions=%s audioOptions=%s",
		len(devices.Video), len(devices.Audio), ui.videoInput.Selected, ui.audioInput.Selected,
		strings.Join(ui.videoInput.Options, " | "), strings.Join(ui.audioInput.Options, " | "))
	ui.appendLog(fmt.Sprintf("devices: video=%d audio=%d", len(devices.Video), len(devices.Audio)))
}

func (ui *mainUI) playAction() {
	ui.openPlayer(false)
}

func (ui *mainUI) playStatAction() {
	parsedURL, err := url.Parse(playStatURL)
	if err != nil {
		ui.showError(fmt.Errorf("invalid play stat URL: %w", err))
		return
	}
	if err := fyne.CurrentApp().OpenURL(parsedURL); err != nil {
		ui.showError(err)
		return
	}
	ui.appendLog("opened play stat: " + playStatURL)
}

func (ui *mainUI) openPlayer(showStat bool) {
	if ui.player == nil {
		ui.showError(fmt.Errorf("ABR player URL unavailable"))
		return
	}
	playerURL := ui.player.URLForConfig(ui.readPreviewConfig())
	if playerURL == "" {
		ui.showError(fmt.Errorf("ABR player URL unavailable"))
		return
	}
	parsedURL, err := url.Parse(playerURL)
	if err != nil {
		ui.showError(fmt.Errorf("invalid player URL: %w", err))
		return
	}
	if showStat {
		q := parsedURL.Query()
		q.Set("stat", "1")
		parsedURL.RawQuery = q.Encode()
		playerURL = parsedURL.String()
	}
	if err := fyne.CurrentApp().OpenURL(parsedURL); err != nil {
		ui.showError(err)
		return
	}
	ui.appendLog("opened player: " + playerURL)
}

func (ui *mainUI) startAction() {
	_ = ui.startPublisher(true)
}

func (ui *mainUI) startPublisher(showDialog bool) error {
	if ui.running.Load() {
		ui.updatePublisherButtons()
		return nil
	}
	if ui.startButton != nil {
		ui.startButton.Disable()
	}
	cfg, err := ui.readConfig()
	if err != nil {
		ui.updatePublisherButtons()
		ui.reportPublisherError(err, showDialog)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(ui.configPath), 0750); err != nil {
		ui.updatePublisherButtons()
		ui.reportPublisherError(err, showDialog)
		return err
	}
	if err := conf.Save(ui.configPath, cfg); err != nil {
		ui.updatePublisherButtons()
		ui.reportPublisherError(err, showDialog)
		return err
	}
	targets := buildPublishTargets(cfg)
	ffCfg := ws.FFmpegConfig{
		InputFile:   cfg.InputSpec(),
		TargetURLs:  targets,
		VideoLayout: cfg.Input.VideoLayout,
		Outputs:     streamOutputs(activeStreamsForInput(cfg)),
	}
	ws.InitFFmpegPublisher(ffCfg)
	ui.setAllPreviewStatus("Starting")
	if err := ws.StartFFmpegPublisher(); err != nil {
		ui.setAllPreviewStatus("Stopped")
		ui.updatePublisherButtons()
		ui.reportPublisherError(err, showDialog)
		return err
	}
	ui.running.Store(true)
	ui.updatePublisherButtons()
	ui.refreshPublisherStatus()
	ui.startStatusPolling()
	ui.appendLog("publisher started")
	ui.appendLog("input: " + cfg.InputSpec())
	for _, target := range targets {
		ui.appendLog("target: " + maskTxSecret(target))
	}
	return nil
}

func (ui *mainUI) reportPublisherError(err error, showDialog bool) {
	if showDialog {
		ui.showError(err)
		return
	}
	if err != nil {
		ui.appendLog("publisher error: " + err.Error())
	}
}

func (ui *mainUI) noteRuntimeConfigChange(field string) {
	if ui == nil || !ui.running.Load() {
		return
	}
	ui.appendLog(field + " changed; stop/start publisher to apply")
}

func (ui *mainUI) stopAction() {
	if !ui.running.Load() {
		ui.updatePublisherButtons()
		return
	}
	if ui.stopButton != nil {
		ui.stopButton.Disable()
	}
	if err := ws.StopFFmpegPublisher(); err != nil {
		ui.updatePublisherButtons()
		ui.showError(err)
		return
	}
	ui.running.Store(false)
	ui.updatePublisherButtons()
	ui.stopStatusPolling()
	ui.setAllPreviewStatus("Stopped")
	ui.appendLog("publisher stopped")
}

func (ui *mainUI) updatePublisherButtons() {
	if ui.startButton == nil || ui.stopButton == nil {
		return
	}
	if ui.running.Load() {
		ui.startButton.Disable()
		ui.stopButton.Enable()
		return
	}
	ui.startButton.Enable()
	ui.stopButton.Disable()
}

func (ui *mainUI) readConfig() (*conf.Config, error) {
	port, err := strconv.Atoi(strings.TrimSpace(ui.srtPort.Text))
	if err != nil {
		return nil, fmt.Errorf("invalid SRT port: %w", err)
	}
	tokenDays, err := strconv.Atoi(strings.TrimSpace(ui.tokenDays.Text))
	if err != nil {
		return nil, fmt.Errorf("invalid token days: %w", err)
	}
	cfg := &conf.Config{
		SiteName: ui.selectedStreamName(),
		TencentSrt: conf.TencentSrt{
			Host:      strings.TrimSpace(ui.srtHost.Text),
			Port:      port,
			App:       strings.TrimSpace(ui.srtApp.Text),
			TokenDays: tokenDays,
		},
		Input: conf.InputConfig{
			Mode:        strings.TrimSpace(ui.inputMode.Selected),
			VideoDevice: strings.TrimSpace(ui.videoInput.Selected),
			AudioDevice: audioDeviceValue(ui.audioInput.Selected),
			VideoFile:   strings.TrimSpace(ui.videoFile.Text),
			VideoLayout: ui.selectedVideoLayout(),
		},
		Publish: conf.PublishConfig{
			OnReady:             ui.onReady.Checked,
			ReconnectMinSeconds: 3,
			ReconnectMaxSeconds: 60,
		},
		Streams: append([]conf.StreamConfig(nil), ui.streams...),
		Secrets: ui.secrets,
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (ui *mainUI) applyConfig(cfg *conf.Config) {
	utils.InitSecrets(cfg.Secrets.AppID, cfg.Secrets.AppSecret, cfg.Secrets.TxKeyMain, cfg.Secrets.TxKeyBack)
	ui.secrets = cfg.Secrets
	ui.streams = append([]conf.StreamConfig(nil), cfg.Streams...)
	siteName := siteNameForStreamName(cfg.SiteName)
	ui.siteName.Options = uniqueSelectOptions(siteNameOptions, siteName)
	ui.siteName.SetSelected(siteName)
	ui.setStreamNameForSite(siteName, cfg.SiteName)
	ui.srtHost.SetText(cfg.TencentSrt.Host)
	ui.srtPort.SetText(strconv.Itoa(cfg.TencentSrt.Port))
	ui.srtApp.SetText(cfg.TencentSrt.App)
	ui.tokenDays.SetText(strconv.Itoa(cfg.TencentSrt.TokenDays))
	if cfg.Input.Mode == "" {
		cfg.Input.Mode = "device"
	}
	ui.inputMode.SetSelected(cfg.Input.Mode)
	ui.updateInputModeFields()
	ui.videoInput.Options = uniqueSelectOptions(ui.videoInput.Options, cfg.Input.VideoDevice)
	ui.videoInput.SetSelected(cfg.Input.VideoDevice)
	audioSelected := audioDisplayValue(cfg.Input.AudioDevice)
	ui.audioInput.Options = uniqueSelectOptions(append([]string{noAudioOption}, ui.audioInput.Options...), audioSelected)
	ui.audioInput.SetSelected(audioSelected)
	ui.videoFile.SetText(cfg.Input.VideoFile)
	if cfg.Input.VideoLayout == "" {
		cfg.Input.VideoLayout = "portrait"
	}
	if layout := layoutForStreamName(cfg.SiteName); layout != "" {
		cfg.Input.VideoLayout = layout
	}
	ui.layout.SetSelected(cfg.Input.VideoLayout)
	ui.onReady.SetChecked(cfg.Publish.OnReady)
}

func (ui *mainUI) refreshPreview() {
	if ui.preview == nil {
		return
	}
	ui.previews = buildStreamPreviews(ui.readPreviewConfig())
	ui.applyTargetStatusToPreviews()
	ui.preview.Refresh()
}

func (ui *mainUI) refreshPublisherStatus() {
	snapshot := ws.GetPublisherStatusSnapshot()
	next := make(map[string]string, len(snapshot))
	for target, status := range snapshot {
		streamName := streamNameFromTencentURL(target)
		if streamName == "" {
			continue
		}
		next[streamName] = status
	}
	ui.uiMu.Lock()
	ui.targetStatus = next
	ui.uiMu.Unlock()
	ui.refreshPreview()
}

func (ui *mainUI) setAllPreviewStatus(status string) {
	ui.uiMu.Lock()
	if ui.targetStatus == nil {
		ui.targetStatus = make(map[string]string)
	}
	for _, preview := range ui.previews {
		ui.targetStatus[preview.StreamName] = status
	}
	ui.uiMu.Unlock()
	ui.refreshPreview()
}

func (ui *mainUI) applyTargetStatusToPreviews() {
	ui.uiMu.Lock()
	defer ui.uiMu.Unlock()
	for i := range ui.previews {
		status := ui.targetStatus[ui.previews[i].StreamName]
		if status == "" {
			status = "Idle"
		}
		ui.previews[i].Status = status
	}
}

func (ui *mainUI) startStatusPolling() {
	ui.stopStatusPolling()
	stop := make(chan struct{})
	ui.uiMu.Lock()
	ui.statusStop = stop
	ui.uiMu.Unlock()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if !ui.running.Load() {
					return
				}
				ui.refreshPublisherStatus()
			}
		}
	}()
}

func (ui *mainUI) stopStatusPolling() {
	ui.uiMu.Lock()
	stop := ui.statusStop
	ui.statusStop = nil
	ui.uiMu.Unlock()
	if stop == nil {
		return
	}
	close(stop)
}

func (ui *mainUI) readPreviewConfig() *conf.Config {
	port, err := strconv.Atoi(strings.TrimSpace(ui.srtPort.Text))
	if err != nil || port <= 0 {
		port = 9000
	}
	tokenDays, err := strconv.Atoi(strings.TrimSpace(ui.tokenDays.Text))
	if err != nil || tokenDays <= 0 {
		tokenDays = utils.TX_PUBISH_TOKEN_DAYS
	}
	streamName := ui.selectedStreamName()
	if streamName == "" {
		streamName = "streamName"
	}
	host := strings.TrimSpace(ui.srtHost.Text)
	if host == "" {
		host = "publish.example.com"
	}
	appName := strings.TrimSpace(ui.srtApp.Text)
	if appName == "" {
		appName = "live"
	}
	return &conf.Config{
		SiteName: streamName,
		TencentSrt: conf.TencentSrt{
			Host:      host,
			Port:      port,
			App:       appName,
			TokenDays: tokenDays,
		},
		Input: conf.InputConfig{
			Mode:        strings.TrimSpace(ui.inputMode.Selected),
			AudioDevice: audioDeviceValue(ui.audioInput.Selected),
			VideoLayout: ui.selectedVideoLayout(),
		},
		Streams: append([]conf.StreamConfig(nil), ui.streams...),
	}
}

func (ui *mainUI) appendLog(line string) {
	ts := time.Now().Format("15:04:05")
	text := strings.TrimSpace(ui.logs.Text)
	if text != "" {
		text += "\n"
	}
	ui.logs.SetText(text + ts + " " + line)
}

func (ui *mainUI) showError(err error) {
	if err == nil {
		return
	}
	ui.appendLog("error: " + err.Error())
	dialog.ShowError(err, ui.window)
}

func defaultConfig() *conf.Config {
	cfg := &conf.Config{
		SiteName: "3drush-fwh",
		TencentSrt: conf.TencentSrt{
			Host:      "publish.example.com",
			Port:      9000,
			App:       "live",
			TokenDays: utils.TX_PUBISH_TOKEN_DAYS,
		},
		Input: conf.InputConfig{
			Mode:        "device",
			VideoLayout: "portrait",
		},
		Publish: conf.PublishConfig{
			OnReady:             false,
			ReconnectMinSeconds: 3,
			ReconnectMaxSeconds: 60,
		},
	}
	return cfg
}

func siteNameForStreamName(streamName string) string {
	streamName = strings.TrimSpace(streamName)
	if siteName := siteNameByStreamName[streamName]; siteName != "" {
		return siteName
	}
	if streamName != "" {
		return streamName
	}
	return siteNameOptions[0]
}

func defaultStreamNameForSite(siteName string) string {
	options := streamNamesBySiteName[strings.TrimSpace(siteName)]
	if len(options) == 0 {
		return ""
	}
	return options[0]
}

func layoutForStreamName(streamName string) string {
	streamName = strings.ToLower(strings.TrimSpace(streamName))
	switch {
	case strings.HasSuffix(streamName, "fwh"):
		return "landscape"
	case strings.HasSuffix(streamName, "fwv"):
		return "portrait"
	default:
		return ""
	}
}

func buildPublishTargets(cfg *conf.Config) []string {
	streams := activeStreamsForInput(cfg)
	targets := make([]string, 0, len(streams))
	for _, stream := range streams {
		targets = append(targets, utils.BuildTencentSRTURL(
			cfg.TencentSrt.Host,
			cfg.TencentSrt.Port,
			cfg.TencentSrt.App,
			conf.ResolveStreamName(cfg.SiteName, stream.StreamName),
			cfg.TencentSrt.TokenDays,
		))
	}
	return targets
}

func buildStreamPreviews(cfg *conf.Config) []streamPreview {
	streams := activeStreamsForInput(cfg)
	urls := buildPublishTargets(cfg)
	out := make([]streamPreview, 0, len(streams))
	hasAudio := inputHasAudio(cfg)
	for i, stream := range streams {
		name := conf.ResolveStreamName(cfg.SiteName, stream.StreamName)
		out = append(out, streamPreview{
			Level:             stream.Level,
			StreamName:        name,
			EncodingParameter: encodingParameterText(stream, cfg.Input.VideoLayout, hasAudio),
			URL:               urls[i],
			Status:            "Idle",
		})
	}
	return out
}

func encodingParameterText(stream conf.StreamConfig, layout string, hasAudio bool) string {
	if stream.AudioOnly {
		return fmt.Sprintf("audio only, %s %s", shortCodec(stream.AudioCodec), stream.AudioBitrate)
	}
	width, height := stream.PortraitWidth, stream.PortraitHeight
	if strings.EqualFold(strings.TrimSpace(layout), "landscape") {
		width, height = stream.LandscapeWidth, stream.LandscapeHeight
	}
	if !hasAudio {
		return fmt.Sprintf("%s %s max %s, %dx%d, no audio",
			shortCodec(stream.VideoCodec),
			stream.VideoBitrate,
			stream.VideoMaxrate,
			width,
			height,
		)
	}
	return fmt.Sprintf("%s %s max %s, %dx%d, audio %s %s",
		shortCodec(stream.VideoCodec),
		stream.VideoBitrate,
		stream.VideoMaxrate,
		width,
		height,
		shortCodec(stream.AudioCodec),
		stream.AudioBitrate,
	)
}

func shortCodec(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "h264", "libx264", "h264_qsv":
		return "H.264"
	case "hevc", "h265", "libx265", "hevc_qsv":
		return "HEVC"
	case "libopus", "opus":
		return "Opus"
	case "aac":
		return "AAC"
	default:
		if strings.TrimSpace(codec) == "" {
			return "-"
		}
		return strings.TrimSpace(codec)
	}
}

func buildStreamNames(cfg *conf.Config) []string {
	streams := normalizedStreams(cfg)
	out := make([]string, 0, len(streams))
	for _, stream := range streams {
		out = append(out, conf.ResolveStreamName(cfg.SiteName, stream.StreamName))
	}
	return out
}

func normalizedStreams(cfg *conf.Config) []conf.StreamConfig {
	if cfg == nil || len(cfg.Streams) == 0 {
		return conf.DefaultStreams()
	}
	return conf.NormalizeStreams(cfg.Streams)
}

func activeStreamsForInput(cfg *conf.Config) []conf.StreamConfig {
	streams := normalizedStreams(cfg)
	if inputHasAudio(cfg) {
		return streams
	}
	out := make([]conf.StreamConfig, 0, len(streams))
	for _, stream := range streams {
		if stream.AudioOnly {
			continue
		}
		out = append(out, stream)
	}
	return out
}

func inputHasAudio(cfg *conf.Config) bool {
	if cfg == nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Input.Mode), "device") {
		return strings.TrimSpace(cfg.Input.AudioDevice) != ""
	}
	return true
}

func audioDeviceValue(selected string) string {
	selected = strings.TrimSpace(selected)
	if selected == noAudioOption {
		return ""
	}
	return selected
}

func audioDisplayValue(device string) string {
	device = strings.TrimSpace(device)
	if device == "" {
		return noAudioOption
	}
	return device
}

func streamOutputs(streams []conf.StreamConfig) []ws.OutputProfile {
	out := make([]ws.OutputProfile, 0, len(streams))
	for _, stream := range streams {
		out = append(out, ws.OutputProfile{
			Level:           stream.Level,
			AudioOnly:       stream.AudioOnly,
			VideoCodec:      stream.VideoCodec,
			VideoBitrate:    stream.VideoBitrate,
			VideoMaxrate:    stream.VideoMaxrate,
			PortraitWidth:   stream.PortraitWidth,
			PortraitHeight:  stream.PortraitHeight,
			LandscapeWidth:  stream.LandscapeWidth,
			LandscapeHeight: stream.LandscapeHeight,
			AudioCodec:      stream.AudioCodec,
			AudioBitrate:    stream.AudioBitrate,
		})
	}
	return out
}

func uniqueSelectOptions(options []string, selected string) []string {
	selected = strings.TrimSpace(selected)
	seen := make(map[string]struct{}, len(options)+1)
	out := make([]string, 0, len(options)+1)
	for _, option := range options {
		if _, ok := seen[option]; ok {
			continue
		}
		seen[option] = struct{}{}
		out = append(out, option)
	}
	if selected != "" {
		if _, ok := seen[selected]; !ok {
			out = append([]string{selected}, out...)
		}
	}
	return out
}

func maskTxSecret(raw string) string {
	idx := strings.Index(raw, "txSecret=")
	if idx < 0 {
		return raw
	}
	start := idx + len("txSecret=")
	end := start
	for end < len(raw) && raw[end] != ',' && raw[end] != '&' {
		end++
	}
	secret := raw[start:end]
	if len(secret) <= 10 {
		return raw[:start] + "***" + raw[end:]
	}
	return raw[:start] + secret[:6] + "***" + secret[len(secret)-4:] + raw[end:]
}

func streamNameFromTencentURL(raw string) string {
	idx := strings.Index(raw, "r=")
	if idx < 0 {
		return ""
	}
	value := raw[idx+len("r="):]
	if end := strings.IndexAny(value, ",&"); end >= 0 {
		value = value[:end]
	}
	if slash := strings.LastIndex(value, "/"); slash >= 0 {
		value = value[slash+1:]
	}
	return strings.TrimSpace(value)
}

func statusColor(status string) color.Color {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running":
		return statusRunningColor
	case "starting":
		return statusStartingColor
	case "retrying":
		return statusStartingColor
	case "stopped":
		return statusStoppedColor
	default:
		return statusIdleColor
	}
}

package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	SiteName   string         `yaml:"siteName"`
	TencentSrt TencentSrt     `yaml:"tencentSrt"`
	Input      InputConfig    `yaml:"input"`
	Streams    []StreamConfig `yaml:"streams"`
	Publish    PublishConfig  `yaml:"publish"`
	Secrets    SecretsConfig  `yaml:"secrets"`

	// Legacy fields are kept only to migrate older pusher.local.yml files.
	WorkerType     string `yaml:"workerType,omitempty"`
	WorkerID       int    `yaml:"workerId,omitempty"`
	WorkerRegion   string `yaml:"workerRegion,omitempty"`
	WorkerMgrAddr  string `yaml:"workerMgrAddr,omitempty"`
	AuthAPIAddr    string `yaml:"authApiAddr,omitempty"`
	AppSecret      string `yaml:"appSecret,omitempty"`
	PublishURL     string `yaml:"publishUrl,omitempty"`
	PublishURL2    string `yaml:"publishUrl2,omitempty"`
	InputFile      string `yaml:"inputFile,omitempty"`
	VideoLayout    string `yaml:"videoLayout,omitempty"`
	PublishOnReady bool   `yaml:"publishOnReady,omitempty"`
}

type SecretsConfig struct {
	AppID     string `yaml:"appId"`
	AppSecret string `yaml:"appSecret"`
	TxKeyMain string `yaml:"txKeyMain"`
	TxKeyBack string `yaml:"txKeyBack"`
}

type TencentSrt struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	App       string `yaml:"app"`
	TokenDays int    `yaml:"tokenDays"`
}

type InputConfig struct {
	Mode        string `yaml:"mode"`
	VideoDevice string `yaml:"videoDevice"`
	AudioDevice string `yaml:"audioDevice"`
	VideoFile   string `yaml:"videoFile"`
	VideoLayout string `yaml:"videoLayout"`
}

type PublishConfig struct {
	OnReady             bool `yaml:"onReady"`
	ReconnectMinSeconds int  `yaml:"reconnectMinSeconds"`
	ReconnectMaxSeconds int  `yaml:"reconnectMaxSeconds"`
}

type StreamConfig struct {
	Level           string `yaml:"level"`
	StreamName      string `yaml:"streamName"`
	AudioOnly       bool   `yaml:"audioOnly"`
	VideoCodec      string `yaml:"videoCodec,omitempty"`
	VideoBitrate    string `yaml:"videoBitrate,omitempty"`
	VideoMaxrate    string `yaml:"videoMaxrate,omitempty"`
	PortraitWidth   int    `yaml:"portraitWidth,omitempty"`
	PortraitHeight  int    `yaml:"portraitHeight,omitempty"`
	LandscapeWidth  int    `yaml:"landscapeWidth,omitempty"`
	LandscapeHeight int    `yaml:"landscapeHeight,omitempty"`
	AudioCodec      string `yaml:"audioCodec"`
	AudioBitrate    string `yaml:"audioBitrate"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse yaml %q: %w", path, err)
	}

	cfg.applyLegacyFields()
	overrideByEnv(cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func Save(path string, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal yaml %q: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config %q: %w", path, err)
	}
	return nil
}

func (c *Config) Validate() error {
	c.SiteName = strings.TrimSpace(c.SiteName)
	if c.SiteName == "" {
		return fmt.Errorf("siteName is required")
	}
	if !isSafeSiteName(c.SiteName) {
		return fmt.Errorf("siteName can only contain letters, numbers, '_' and '-'")
	}
	c.TencentSrt.Host = strings.TrimSpace(c.TencentSrt.Host)
	if c.TencentSrt.Host == "" {
		return fmt.Errorf("tencentSrt.host is required")
	}
	if c.TencentSrt.Port <= 0 || c.TencentSrt.Port > 65535 {
		return fmt.Errorf("tencentSrt.port must be 1-65535")
	}
	c.TencentSrt.App = strings.Trim(strings.TrimSpace(c.TencentSrt.App), "/")
	if c.TencentSrt.App == "" {
		return fmt.Errorf("tencentSrt.app is required")
	}
	if c.TencentSrt.TokenDays <= 0 {
		c.TencentSrt.TokenDays = 30
	}
	c.Input.Mode = strings.ToLower(strings.TrimSpace(c.Input.Mode))
	if c.Input.Mode == "" {
		c.Input.Mode = "device"
	}
	switch c.Input.Mode {
	case "device", "file":
	default:
		return fmt.Errorf("input.mode must be one of: device, file")
	}
	c.Input.VideoDevice = strings.TrimSpace(c.Input.VideoDevice)
	c.Input.AudioDevice = strings.TrimSpace(c.Input.AudioDevice)
	c.Input.VideoFile = strings.TrimSpace(c.Input.VideoFile)
	if c.Input.Mode == "file" {
		if c.Input.VideoFile == "" {
			return fmt.Errorf("input.videoFile is required when input.mode=file")
		}
		if !strings.EqualFold(strings.ToLower(filepath.Ext(c.Input.VideoFile)), ".mp4") {
			return fmt.Errorf("input.videoFile must be an .mp4 file")
		}
	} else {
		if c.Input.VideoDevice == "" {
			return fmt.Errorf("input.videoDevice is required when input.mode=device")
		}
	}
	c.Input.VideoLayout = strings.ToLower(strings.TrimSpace(c.Input.VideoLayout))
	if c.Input.VideoLayout == "" {
		c.Input.VideoLayout = "portrait"
	}
	switch c.Input.VideoLayout {
	case "portrait", "landscape":
	default:
		return fmt.Errorf("input.videoLayout must be one of: portrait, landscape")
	}
	if c.Publish.ReconnectMinSeconds <= 0 {
		c.Publish.ReconnectMinSeconds = 3
	}
	if c.Publish.ReconnectMaxSeconds <= 0 {
		c.Publish.ReconnectMaxSeconds = 60
	}
	if c.Publish.ReconnectMaxSeconds < c.Publish.ReconnectMinSeconds {
		c.Publish.ReconnectMaxSeconds = c.Publish.ReconnectMinSeconds
	}
	c.Streams = NormalizeStreams(c.Streams)
	if err := validateStreams(c.SiteName, c.Streams); err != nil {
		return err
	}
	if err := c.Secrets.Validate(); err != nil {
		return err
	}
	return nil
}

func (s *SecretsConfig) Validate() error {
	s.AppID = strings.TrimSpace(s.AppID)
	s.AppSecret = strings.TrimSpace(s.AppSecret)
	s.TxKeyMain = strings.TrimSpace(s.TxKeyMain)
	s.TxKeyBack = strings.TrimSpace(s.TxKeyBack)
	if s.AppID == "" {
		return fmt.Errorf("secrets.appId is required (set in config or VPUBLISHER_APP_ID)")
	}
	if s.AppSecret == "" {
		return fmt.Errorf("secrets.appSecret is required (set in config or VPUBLISHER_APP_SECRET)")
	}
	if s.TxKeyMain == "" {
		return fmt.Errorf("secrets.txKeyMain is required (set in config or VPUBLISHER_TX_KEY_MAIN)")
	}
	if s.TxKeyBack == "" {
		return fmt.Errorf("secrets.txKeyBack is required (set in config or VPUBLISHER_TX_KEY_BACK)")
	}
	return nil
}

func DefaultStreams() []StreamConfig {
	return []StreamConfig{
		{
			Level:        "bottom",
			StreamName:   "{siteName}_audio",
			AudioOnly:    true,
			AudioCodec:   "aac",
			AudioBitrate: "128k",
		},
		{
			Level:           "economic",
			StreamName:      "{siteName}_economic",
			VideoCodec:      "h264",
			VideoBitrate:    "400k",
			VideoMaxrate:    "600k",
			PortraitWidth:   360,
			PortraitHeight:  640,
			LandscapeWidth:  640,
			LandscapeHeight: 360,
			AudioCodec:      "aac",
			AudioBitrate:    "128k",
		},
		{
			Level:           "standard_hevc",
			StreamName:      "{siteName}_standard_hevc",
			VideoCodec:      "hevc",
			VideoBitrate:    "600k",
			VideoMaxrate:    "1000k",
			PortraitWidth:   720,
			PortraitHeight:  1280,
			LandscapeWidth:  1280,
			LandscapeHeight: 720,
			AudioCodec:      "aac",
			AudioBitrate:    "128k",
		},
		{
			Level:           "standard",
			StreamName:      "{siteName}_standard",
			VideoCodec:      "h264",
			VideoBitrate:    "1000k",
			VideoMaxrate:    "1500k",
			PortraitWidth:   720,
			PortraitHeight:  1280,
			LandscapeWidth:  1280,
			LandscapeHeight: 720,
			AudioCodec:      "aac",
			AudioBitrate:    "128k",
		},
		{
			Level:           "high",
			StreamName:      "{siteName}",
			VideoCodec:      "h264",
			VideoBitrate:    "2000k",
			VideoMaxrate:    "3000k",
			PortraitWidth:   1080,
			PortraitHeight:  1920,
			LandscapeWidth:  1920,
			LandscapeHeight: 1080,
			AudioCodec:      "aac",
			AudioBitrate:    "128k",
		},
	}
}

func overrideByEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("VPUBLISHER_SITE_NAME")); v != "" {
		cfg.SiteName = v
	}
	if v := strings.TrimSpace(os.Getenv("VPUBLISHER_VIDEO_LAYOUT")); v != "" {
		cfg.Input.VideoLayout = v
	}
	if v := strings.TrimSpace(os.Getenv("VPUBLISHER_PUBLISH_ON_READY")); v != "" {
		if enabled, err := strconv.ParseBool(v); err == nil {
			cfg.Publish.OnReady = enabled
		}
	}
	if v := strings.TrimSpace(os.Getenv("VPUBLISHER_APP_ID")); v != "" {
		cfg.Secrets.AppID = v
	}
	if v := strings.TrimSpace(os.Getenv("VPUBLISHER_APP_SECRET")); v != "" {
		cfg.Secrets.AppSecret = v
	}
	if v := strings.TrimSpace(os.Getenv("VPUBLISHER_TX_KEY_MAIN")); v != "" {
		cfg.Secrets.TxKeyMain = v
	}
	if v := strings.TrimSpace(os.Getenv("VPUBLISHER_TX_KEY_BACK")); v != "" {
		cfg.Secrets.TxKeyBack = v
	}
}

func NormalizeStreams(streams []StreamConfig) []StreamConfig {
	defaults := DefaultStreams()
	if len(streams) == 0 {
		return defaults
	}
	byLevel := make(map[string]StreamConfig, len(streams))
	for _, stream := range streams {
		level := strings.ToLower(strings.TrimSpace(stream.Level))
		if level == "" {
			continue
		}
		stream.Level = level
		byLevel[level] = stream
	}
	out := make([]StreamConfig, 0, len(defaults))
	for _, def := range defaults {
		stream, ok := byLevel[def.Level]
		if !ok {
			out = append(out, def)
			continue
		}
		stream.Level = def.Level
		stream.StreamName = strings.TrimSpace(stream.StreamName)
		if stream.StreamName == "" {
			stream.StreamName = def.StreamName
		}
		stream.VideoCodec = strings.ToLower(strings.TrimSpace(stream.VideoCodec))
		if stream.VideoCodec == "" {
			stream.VideoCodec = def.VideoCodec
		}
		stream.VideoBitrate = strings.TrimSpace(stream.VideoBitrate)
		if stream.VideoBitrate == "" {
			stream.VideoBitrate = def.VideoBitrate
		}
		stream.VideoMaxrate = strings.TrimSpace(stream.VideoMaxrate)
		if stream.VideoMaxrate == "" {
			stream.VideoMaxrate = def.VideoMaxrate
		}
		if stream.PortraitWidth <= 0 {
			stream.PortraitWidth = def.PortraitWidth
		}
		if stream.PortraitHeight <= 0 {
			stream.PortraitHeight = def.PortraitHeight
		}
		if stream.LandscapeWidth <= 0 {
			stream.LandscapeWidth = def.LandscapeWidth
		}
		if stream.LandscapeHeight <= 0 {
			stream.LandscapeHeight = def.LandscapeHeight
		}
		stream.AudioCodec = normalizeAudioCodec(stream.AudioCodec)
		if stream.AudioCodec == "" {
			stream.AudioCodec = def.AudioCodec
		}
		stream.AudioBitrate = strings.TrimSpace(stream.AudioBitrate)
		if stream.AudioBitrate == "" {
			stream.AudioBitrate = def.AudioBitrate
		}
		if def.AudioOnly {
			stream.AudioOnly = true
		}
		out = append(out, stream)
	}
	return out
}

func ResolveStreamName(siteName, streamName string) string {
	streamName = strings.TrimSpace(streamName)
	if streamName == "" {
		return strings.TrimSpace(siteName)
	}
	replacer := strings.NewReplacer(
		"{siteName}", strings.TrimSpace(siteName),
		"{sitename}", strings.TrimSpace(siteName),
		"${siteName}", strings.TrimSpace(siteName),
		"${sitename}", strings.TrimSpace(siteName),
		"{site}", strings.TrimSpace(siteName),
		"${site}", strings.TrimSpace(siteName),
	)
	return replacer.Replace(streamName)
}

func normalizeAudioCodec(codec string) string {
	codec = strings.ToLower(strings.TrimSpace(codec))
	switch codec {
	case "libopus", "opus":
		return "aac"
	default:
		return codec
	}
}

var bitratePattern = regexp.MustCompile(`^[1-9][0-9]*[kKmM]?$`)

func validateStreams(siteName string, streams []StreamConfig) error {
	if len(streams) != 5 {
		return fmt.Errorf("streams must contain exactly five levels")
	}
	seen := make(map[string]struct{}, len(streams))
	for _, stream := range streams {
		resolvedName := ResolveStreamName(siteName, stream.StreamName)
		if resolvedName == "" {
			return fmt.Errorf("streams.%s.streamName is required", stream.Level)
		}
		if !isSafeSiteName(resolvedName) {
			return fmt.Errorf("streams.%s.streamName resolves to invalid stream name %q", stream.Level, resolvedName)
		}
		if _, ok := seen[resolvedName]; ok {
			return fmt.Errorf("streams.%s.streamName resolves to duplicate stream name %q", stream.Level, resolvedName)
		}
		seen[resolvedName] = struct{}{}

		switch stream.AudioCodec {
		case "aac":
		default:
			return fmt.Errorf("streams.%s.audioCodec must be aac for Tencent SRT playback compatibility", stream.Level)
		}
		if !bitratePattern.MatchString(stream.AudioBitrate) {
			return fmt.Errorf("streams.%s.audioBitrate is invalid: %q", stream.Level, stream.AudioBitrate)
		}
		if stream.AudioOnly {
			continue
		}
		switch stream.VideoCodec {
		case "h264", "libx264", "h264_qsv", "h264_nvenc", "hevc", "h265", "libx265", "hevc_qsv", "hevc_nvenc":
		default:
			return fmt.Errorf("streams.%s.videoCodec must be one of: h264, libx264, h264_qsv, h264_nvenc, hevc, h265, libx265, hevc_qsv, hevc_nvenc", stream.Level)
		}
		if !bitratePattern.MatchString(stream.VideoBitrate) {
			return fmt.Errorf("streams.%s.videoBitrate is invalid: %q", stream.Level, stream.VideoBitrate)
		}
		if !bitratePattern.MatchString(stream.VideoMaxrate) {
			return fmt.Errorf("streams.%s.videoMaxrate is invalid: %q", stream.Level, stream.VideoMaxrate)
		}
		if !isPositiveEven(stream.PortraitWidth) || !isPositiveEven(stream.PortraitHeight) ||
			!isPositiveEven(stream.LandscapeWidth) || !isPositiveEven(stream.LandscapeHeight) {
			return fmt.Errorf("streams.%s dimensions must be positive even numbers", stream.Level)
		}
	}
	return nil
}

func isPositiveEven(v int) bool {
	return v > 0 && v%2 == 0
}

func (c *Config) applyLegacyFields() {
	if c.SiteName == "" {
		for _, raw := range []string{c.PublishURL, c.PublishURL2} {
			if site := extractLegacySiteName(raw); site != "" {
				c.SiteName = site
				break
			}
		}
	}
	if c.Input.VideoLayout == "" && c.VideoLayout != "" {
		c.Input.VideoLayout = c.VideoLayout
	}
	if c.Input.VideoDevice == "" && c.InputFile != "" {
		if strings.EqualFold(filepath.Ext(c.InputFile), ".mp4") {
			c.Input.Mode = "file"
			c.Input.VideoFile = c.InputFile
		} else {
			video, audio := ParseInputSpec(c.InputFile)
			c.Input.VideoDevice = video
			c.Input.AudioDevice = audio
		}
	}
	if !c.Publish.OnReady && c.PublishOnReady {
		c.Publish.OnReady = true
	}
	if c.TencentSrt.Host == "" {
		c.TencentSrt.Host = "publish.example.com"
	}
	if c.TencentSrt.Port == 0 {
		c.TencentSrt.Port = 9000
	}
	if c.TencentSrt.App == "" {
		c.TencentSrt.App = "live"
	}
}

func (c *Config) InputSpec() string {
	if c.Input.Mode == "file" {
		return strings.TrimSpace(c.Input.VideoFile)
	}
	parts := make([]string, 0, 2)
	if strings.TrimSpace(c.Input.VideoDevice) != "" {
		parts = append(parts, `video="`+strings.TrimSpace(c.Input.VideoDevice)+`"`)
	}
	if strings.TrimSpace(c.Input.AudioDevice) != "" {
		parts = append(parts, `audio="`+strings.TrimSpace(c.Input.AudioDevice)+`"`)
	}
	return strings.Join(parts, ":")
}

func ParseInputSpec(input string) (string, string) {
	var videoName string
	var audioName string
	parts := strings.Split(input, ":")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		lower := strings.ToLower(part)
		switch {
		case strings.HasPrefix(lower, "video="):
			videoName = trimQuotedDevice(part[len("video="):])
		case strings.HasPrefix(lower, "audio="):
			audioName = trimQuotedDevice(part[len("audio="):])
		}
	}
	return videoName, audioName
}

func trimQuotedDevice(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
		return strings.TrimSpace(v[1 : len(v)-1])
	}
	return v
}

func isSafeSiteName(v string) bool {
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

func extractLegacySiteName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	idx := strings.Index(raw, "/")
	if idx < 0 {
		return ""
	}
	parts := strings.Split(raw, "/")
	last := parts[len(parts)-1]
	if q := strings.Index(last, "?"); q >= 0 {
		last = last[:q]
	}
	if c := strings.Index(last, ":"); c >= 0 {
		last = last[:c]
	}
	return strings.TrimSpace(last)
}

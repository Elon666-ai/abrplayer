package conf

import "testing"

func TestValidateRejectsDuplicateResolvedStreams(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Streams = DefaultStreams()
	cfg.Streams[1].StreamName = "{siteName}"
	cfg.Streams[3].StreamName = "{siteName}"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected duplicate stream error")
	}
}

func TestValidateRejectsInvalidStreamBitrate(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Streams = DefaultStreams()
	cfg.Streams[1].VideoBitrate = "fast"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected bitrate error")
	}
}

func TestValidateRejectsOddStreamDimensions(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Streams = DefaultStreams()
	cfg.Streams[1].PortraitWidth = 361
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected dimension error")
	}
}

func TestNormalizeStreamsCoercesLegacyOpusAudioCodecToAAC(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Streams = DefaultStreams()
	for i := range cfg.Streams {
		cfg.Streams[i].AudioCodec = "libopus"
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() expected legacy libopus to be normalized, got %v", err)
	}
	for _, stream := range cfg.Streams {
		if stream.AudioCodec != "aac" {
			t.Fatalf("AudioCodec expected aac after normalization, got %q", stream.AudioCodec)
		}
	}
}
func TestValidateAllowsDeviceInputWithoutAudio(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Input = InputConfig{
		Mode:        "device",
		VideoDevice: "OBS Virtual Camera",
		VideoLayout: "portrait",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() expected nil error, got %v", err)
	}
	if cfg.Input.AudioDevice != "" {
		t.Fatalf("AudioDevice expected empty, got %q", cfg.Input.AudioDevice)
	}
}

func minimalValidConfig() *Config {
	return &Config{
		SiteName: "demo",
		TencentSrt: TencentSrt{
			Host:      "publish.example.com",
			Port:      9000,
			App:       "live",
			TokenDays: 30,
		},
		Input: InputConfig{
			Mode:        "file",
			VideoFile:   "D:/video/test.mp4",
			VideoLayout: "portrait",
		},
		Publish: PublishConfig{
			ReconnectMinSeconds: 3,
			ReconnectMaxSeconds: 60,
		},
		Secrets: SecretsConfig{
			AppID:     "test-app-id",
			AppSecret: "test-app-secret",
			TxKeyMain: "test-tx-main",
			TxKeyBack: "test-tx-back",
		},
	}
}

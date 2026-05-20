package ws

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNormalizeOutputProfilesPreservesMissingAudioOnlyLevel(t *testing.T) {
	outputs := []OutputProfile{
		{Level: "economic"},
		{Level: "standard_hevc"},
		{Level: "standard"},
		{Level: "high"},
	}

	normalized := normalizeOutputProfiles(outputs, "portrait")
	if len(normalized) != 4 {
		t.Fatalf("normalizeOutputProfiles() length = %d, want 4", len(normalized))
	}
	if normalized[0].AudioOnly {
		t.Fatalf("economic output was marked audio-only: %+v", normalized[0])
	}
	if normalized[0].VideoBitrate == "" || normalized[0].PortraitWidth == 0 {
		t.Fatalf("economic defaults were not applied: %+v", normalized[0])
	}
}

func TestAppendFourLevelOutputsUsesRecoverableFifoMuxer(t *testing.T) {
	args := appendFourLevelOutputs(nil, "portrait", []string{
		"srt://example/audio",
		"srt://example/economic",
		"srt://example/standard_hevc",
		"srt://example/standard",
		"srt://example/high",
	}, nil, configuredFFmpegEncoderConfig("software"), "0:a:0?")

	if got := countArgs(args, "fifo"); got != 5 {
		t.Fatalf("fifo muxer count = %d, want 5 in args %v", got, args)
	}
	if !slices.Contains(args, "-attempt_recovery") || !slices.Contains(args, "-recover_any_error") {
		t.Fatalf("recoverable fifo options missing from args %v", args)
	}
	if !slices.Contains(args, "aac") {
		t.Fatalf("default Tencent SRT audio codec should be AAC, args %v", args)
	}
}

func TestAppendFourLevelOutputsUsesNVENCWhenConfigured(t *testing.T) {
	args := appendFourLevelOutputs(nil, "portrait", []string{
		"srt://example/audio",
		"srt://example/economic",
		"srt://example/standard_hevc",
		"srt://example/standard",
		"srt://example/high",
	}, []OutputProfile{
		{Level: "audio", AudioOnly: true, AudioCodec: "aac", AudioBitrate: "128k"},
		{Level: "economic", VideoCodec: "h264_qsv", VideoBitrate: "400k", VideoMaxrate: "600k", AudioCodec: "aac", AudioBitrate: "128k"},
		{Level: "standard_hevc", VideoCodec: "hevc_qsv", VideoBitrate: "1000k", VideoMaxrate: "1500k", AudioCodec: "aac", AudioBitrate: "128k"},
		{Level: "standard", VideoCodec: "h264_qsv", VideoBitrate: "1000k", VideoMaxrate: "1500k", AudioCodec: "aac", AudioBitrate: "128k"},
		{Level: "high", VideoCodec: "h264_qsv", VideoBitrate: "2000k", VideoMaxrate: "3000k", AudioCodec: "aac", AudioBitrate: "128k"},
	}, configuredFFmpegEncoderConfig("nvenc"), "0:a:0?")

	if slices.Contains(args, "h264_qsv") || slices.Contains(args, "hevc_qsv") {
		t.Fatalf("qsv encoder leaked into nvenc args %v", args)
	}
	if !slices.Contains(args, "h264_nvenc") || !slices.Contains(args, "hevc_nvenc") {
		t.Fatalf("nvenc encoders missing from args %v", args)
	}
}

func TestFFmpegAudioMapRequiresConfiguredDShowAudio(t *testing.T) {
	got := ffmpegAudioMap(`video=OBS Virtual Camera:audio=Line (CQ12T)`)
	if got != "0:a:0" {
		t.Fatalf("ffmpegAudioMap() = %q, want mandatory audio map", got)
	}

	got = ffmpegAudioMap(`video=OBS Virtual Camera`)
	if got != "0:a:0?" {
		t.Fatalf("ffmpegAudioMap() = %q, want optional audio map", got)
	}
}

func TestAppendVideoRenditionsOmitsH264ProfileForHEVC(t *testing.T) {
	args := appendVideoRenditions(nil, "hevc_nvenc", "portrait")
	if slices.Contains(args, "high") {
		t.Fatalf("h264 profile option leaked into hevc args %v", args)
	}
}

func TestFFmpegExecutableUsesExplicitEnvPath(t *testing.T) {
	dir := t.TempDir()
	name := ffmpegBinaryName()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FFMPEG_PATH", path)

	got, err := ffmpegExecutable()
	if err != nil {
		t.Fatalf("ffmpegExecutable() error = %v", err)
	}
	if got != path {
		t.Fatalf("ffmpegExecutable() = %q, want %q", got, path)
	}
}

func countArgs(args []string, value string) int {
	var n int
	for _, arg := range args {
		if arg == value {
			n++
		}
	}
	return n
}

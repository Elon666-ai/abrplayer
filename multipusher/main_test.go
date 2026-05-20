package main

import (
	"testing"

	"vpublisher/conf"
)

func TestActiveStreamsForInputSkipsAudioOnlyWhenNoAudioDevice(t *testing.T) {
	cfg := &conf.Config{
		SiteName: "demo",
		Input: conf.InputConfig{
			Mode:        "device",
			VideoDevice: "OBS Virtual Camera",
			VideoLayout: "portrait",
		},
		Streams: conf.DefaultStreams(),
	}

	streams := activeStreamsForInput(cfg)
	if len(streams) != 4 {
		t.Fatalf("activeStreamsForInput() length = %d, want 4", len(streams))
	}
	for _, stream := range streams {
		if stream.AudioOnly {
			t.Fatalf("activeStreamsForInput() included audio-only stream: %+v", stream)
		}
	}
	targets := buildPublishTargets(cfg)
	if len(targets) != 4 {
		t.Fatalf("buildPublishTargets() length = %d, want 4", len(targets))
	}
	outputs := streamOutputs(streams)
	if len(outputs) != 4 {
		t.Fatalf("streamOutputs() length = %d, want 4", len(outputs))
	}
	if outputs[0].AudioOnly {
		t.Fatalf("first video output was marked audio-only: %+v", outputs[0])
	}
}

func TestDefaultConfigDoesNotPublishOnReady(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Publish.OnReady {
		t.Fatal("defaultConfig() Publish.OnReady = true, want false")
	}
}

func TestLayoutForStreamNameUsesFwhFwvRule(t *testing.T) {
	tests := []struct {
		streamName string
		want       string
	}{
		{streamName: "3drush-fwh", want: "landscape"},
		{streamName: "gsp2w-fwh", want: "landscape"},
		{streamName: "3drush-fwv", want: "portrait"},
		{streamName: "gsp2w-fwv", want: "portrait"},
		{streamName: "custom", want: ""},
	}

	for _, tt := range tests {
		if got := layoutForStreamName(tt.streamName); got != tt.want {
			t.Fatalf("layoutForStreamName(%q) = %q, want %q", tt.streamName, got, tt.want)
		}
	}
}

package main

import (
	"net/url"
	"testing"

	"vpublisher/conf"
)

func TestPlayerURLForSite(t *testing.T) {
	got := playerURLForSite("http://127.0.0.1:8080/", "3drush-fwv")
	want := "http://127.0.0.1:8080/?site=3drush-fwv"
	if got != want {
		t.Fatalf("playerURLForSite() = %q, want %q", got, want)
	}
}

func TestPlayerURLForStreams(t *testing.T) {
	cfg := &conf.Config{
		SiteName: "demo",
		Streams: []conf.StreamConfig{
			{Level: "bottom", StreamName: "demo_low_audio", AudioOnly: true, AudioCodec: "aac", AudioBitrate: "128k"},
			{Level: "economic", StreamName: "demo_low", VideoCodec: "h264", VideoBitrate: "400k", VideoMaxrate: "600k", PortraitWidth: 360, PortraitHeight: 640, LandscapeWidth: 640, LandscapeHeight: 360, AudioCodec: "aac", AudioBitrate: "128k"},
			{Level: "standard_hevc", StreamName: "demo_mid_hevc", VideoCodec: "hevc", VideoBitrate: "600k", VideoMaxrate: "1000k", PortraitWidth: 720, PortraitHeight: 1280, LandscapeWidth: 1280, LandscapeHeight: 720, AudioCodec: "aac", AudioBitrate: "128k"},
			{Level: "standard", StreamName: "demo_mid", VideoCodec: "h264", VideoBitrate: "1000k", VideoMaxrate: "1500k", PortraitWidth: 720, PortraitHeight: 1280, LandscapeWidth: 1280, LandscapeHeight: 720, AudioCodec: "aac", AudioBitrate: "128k"},
			{Level: "high", StreamName: "demo_top", VideoCodec: "h264", VideoBitrate: "2000k", VideoMaxrate: "3000k", PortraitWidth: 1080, PortraitHeight: 1920, LandscapeWidth: 1920, LandscapeHeight: 1080, AudioCodec: "aac", AudioBitrate: "128k"},
		},
	}
	got := playerURLForStreams("http://127.0.0.1:8080/", cfg.SiteName, resolvedStreamNames(cfg))
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse player url: %v", err)
	}
	q := u.Query()
	for key, want := range map[string]string{
		"site":         "demo",
		"bottom":       "demo_low_audio",
		"economic":     "demo_low",
		"standard":     "demo_mid",
		"standardHevc": "demo_mid_hevc",
		"high":         "demo_top",
	} {
		if got := q.Get(key); got != want {
			t.Fatalf("query %s = %q, want %q; url=%s", key, got, want, u.String())
		}
	}
}

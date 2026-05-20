package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveStreamingPlayDomainToConfigFilePath(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "backend.local.json")
	input := `{
    "HttpPort": 8088,
    "StreamingPlayDomain": "play.example.com"
}
`
	if err := os.WriteFile(filename, []byte(input), 0664); err != nil {
		t.Fatal(err)
	}

	if err := SaveStreamingPlayDomainToConfigFilePath(filename, "https://play-smoke.example.com/live/demo"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"StreamingPlayDomain": "play-smoke.example.com"`) {
		t.Fatalf("config file was not updated: %s", string(data))
	}
}

func TestNormalizeAppEnvName(t *testing.T) {
	cases := map[string]string{
		"LOCAL": "local",
		"Dev":   "dev",
		"test":  "test",
		"STAG":  "stag",
		"prod":  "prod",
		"uat":   "dev",
	}
	for input, want := range cases {
		got, err := NormalizeAppEnvName(input)
		if err != nil {
			t.Fatalf("%s returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("%s normalized to %s, want %s", input, got, want)
		}
	}
}

func TestNormalizePublicBaseURL(t *testing.T) {
	got, err := NormalizePublicBaseURL(" https://videostat-test.example.com/dashboard/?x=1 ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://videostat-test.example.com" {
		t.Fatalf("normalized URL = %s", got)
	}
}

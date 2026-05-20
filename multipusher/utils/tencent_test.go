package utils

import (
	"strings"
	"testing"
)

func TestBuildTencentSRTURL(t *testing.T) {
	got := BuildTencentSRTURL("publish.example.com", 9000, "live", "3drush-fwv", 30)
	if !strings.HasPrefix(got, "srt://publish.example.com:9000?streamid=#!::h=publish.example.com,r=live/3drush-fwv,txSecret=") {
		t.Fatalf("unexpected tencent srt url: %s", got)
	}
	if !strings.Contains(got, ",txTime=") {
		t.Fatalf("missing txTime: %s", got)
	}
}

package main

import (
	"net/url"
	"strings"

	"vpublisher/conf"
)

const playerBaseURL = "http://127.0.0.1:8080/"

type playerClient struct {
	baseURL string
}

func newPlayerClient() *playerClient {
	return &playerClient{baseURL: playerBaseURL}
}

func (p *playerClient) Close() {}

func (p *playerClient) URLForConfig(cfg *conf.Config) string {
	if p == nil || p.baseURL == "" || cfg == nil {
		return ""
	}
	return playerURLForStreams(p.baseURL, cfg.SiteName, resolvedStreamNames(cfg))
}

func playerURLForSite(baseURL, siteName string) string {
	return playerURLForStreams(baseURL, siteName, nil)
}

func playerURLForStreams(baseURL, siteName string, streamNames map[string]string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	siteName = strings.TrimSpace(siteName)
	if siteName != "" {
		q := u.Query()
		q.Set("site", siteName)
		for _, level := range []string{"bottom", "economic", "standard", "standard_hevc", "high"} {
			if streamName := strings.TrimSpace(streamNames[level]); streamName != "" {
				if level == "standard_hevc" {
					q.Set("standardHevc", streamName)
				} else {
					q.Set(level, streamName)
				}
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func resolvedStreamNames(cfg *conf.Config) map[string]string {
	out := make(map[string]string, len(cfg.Streams))
	for _, stream := range conf.NormalizeStreams(cfg.Streams) {
		out[stream.Level] = conf.ResolveStreamName(cfg.SiteName, stream.StreamName)
	}
	return out
}

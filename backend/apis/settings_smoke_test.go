package apis

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlayTxURLUsesConfiguredDomainFallback(t *testing.T) {
	r := NewRouter("local")
	req := httptest.NewRequest(http.MethodGet, "/api/play/txUrl?stream=gsp2w-fwv", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "webrtc://play.example.com/live/gsp2w-fwv") {
		t.Fatalf("response did not use fallback play domain: %s", w.Body.String())
	}
}

func TestDashboardStaticPageLoads(t *testing.T) {
	r := NewRouter("local")
	req := httptest.NewRequest(http.MethodGet, "/dashboard/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "playDomain") {
		t.Fatalf("dashboard page missing play-domain settings UI")
	}
	if !strings.Contains(w.Body.String(), "envDomainRows") {
		t.Fatalf("dashboard page missing environment-domain settings UI")
	}
}

func TestEnvDomainsEndpointUsesDefaultsWithoutDb(t *testing.T) {
	r := NewRouter("local")
	req := httptest.NewRequest(http.MethodGet, "/api/settings/env-domains", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	for _, want := range []string{"local", "dev", "https://videostat-uat.example.com", "https://videostat-prod.example.com"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("env-domain response missing %q: %s", want, w.Body.String())
		}
	}
}

func TestPublicStatAPIDomainEndpointUsesRunEnvDefault(t *testing.T) {
	r := NewRouter("local")
	req := httptest.NewRequest(http.MethodGet, "/api/settings/statapi-domain", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	for _, want := range []string{"local", "local-env", "http://localhost:8088"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("statapi-domain response missing %q: %s", want, w.Body.String())
		}
	}
}

func TestPublicStatAPIDomainEndpointNormalizesEnvQuery(t *testing.T) {
	r := NewRouter("local")
	req := httptest.NewRequest(http.MethodGet, "/api/settings/statapi-domain?env=UAT", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	for _, want := range []string{"dev", "UAT-env", "https://videostat-uat.example.com"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("statapi-domain response missing %q: %s", want, w.Body.String())
		}
	}
}

func TestPublicHealthRoutes(t *testing.T) {
	r := NewRouter("local")
	for _, path := range []string{"/healthz", "/api/stat/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%s expected status 200, got %d: %s", path, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "version") {
			t.Fatalf("%s response missing version marker: %s", path, w.Body.String())
		}
	}
}

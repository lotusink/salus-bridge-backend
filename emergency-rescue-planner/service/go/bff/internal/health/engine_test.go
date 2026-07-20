package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakePinger struct {
	err error
}

func (f *fakePinger) PingContext(ctx context.Context) error { return f.err }

func newHealthRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/health_check", nil)
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) Response {
	t.Helper()
	var resp Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func TestHealth_AllOK_ReturnsPass(t *testing.T) {
	eng := NewHealthEngine(Config{
		ServiceID:   "salus-bridge-bff",
		Description: "test",
		Version:     "test-v1",
		StartedAt:   time.Now().Add(-30 * time.Second),
		DBPinger:    &fakePinger{},
		EnvSnapshot: EnvSnapshot{
			OpenAIKey: true, AnthropicKey: true, ORSKey: true},
		Routes: []Route{{Method: "GET", Path: "/health_check"}},
	})

	w := httptest.NewRecorder()
	eng.GetHealth(w, newHealthRequest())

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != MediaType {
		t.Errorf("Content-Type: want %q, got %q", MediaType, got)
	}
	resp := decodeBody(t, w)
	if resp.Status != StatusPass {
		t.Errorf("Status: want pass, got %q", resp.Status)
	}
	if resp.UptimeSeconds < 1 {
		t.Errorf("UptimeSeconds: want >=1, got %d", resp.UptimeSeconds)
	}
	if resp.GoVersion == "" {
		t.Error("GoVersion should be populated")
	}
}

func TestHealth_DBPingFail_ReturnsFail503(t *testing.T) {
	pingErr := errors.New("connection refused")
	eng := NewHealthEngine(Config{
		StartedAt: time.Now(),
		DBPinger:  &fakePinger{err: pingErr},
		EnvSnapshot: EnvSnapshot{
			OpenAIKey: true, AnthropicKey: true, ORSKey: true},
	})

	w := httptest.NewRecorder()
	eng.GetHealth(w, newHealthRequest())

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: want 503, got %d", w.Code)
	}
	resp := decodeBody(t, w)
	if resp.Status != StatusFail {
		t.Errorf("Status: want fail, got %q", resp.Status)
	}
	dbCheck := resp.Checks["database:ping"]
	if len(dbCheck) != 1 || dbCheck[0].Status != StatusFail {
		t.Errorf("database:ping check: want one fail item, got %+v", dbCheck)
	}
	if resp.Output != pingErr.Error() {
		t.Errorf("Output: want %q, got %q", pingErr.Error(), resp.Output)
	}
}

func TestHealth_MissingOptionalEnv_ReturnsWarn(t *testing.T) {
	eng := NewHealthEngine(Config{
		StartedAt: time.Now(),
		DBPinger:  &fakePinger{},
		EnvSnapshot: EnvSnapshot{
			OpenAIKey: true, AnthropicKey: true,
			ORSKey: false, // unset triggers warn
		},
	})

	w := httptest.NewRecorder()
	eng.GetHealth(w, newHealthRequest())

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	resp := decodeBody(t, w)
	if resp.Status != StatusWarn {
		t.Errorf("Status: want warn, got %q", resp.Status)
	}
	ors := resp.Checks["ors_api:configured"]
	if len(ors) != 1 || ors[0].Status != StatusWarn {
		t.Errorf("ors_api:configured: want warn, got %+v", ors)
	}
}

func TestHealth_RoutesSorted(t *testing.T) {
	eng := NewHealthEngine(Config{
		StartedAt: time.Now(),
		DBPinger:  &fakePinger{},
		Routes: []Route{
			{Method: "POST", Path: "/api/zeta"},
			{Method: "GET", Path: "/api/alpha"},
			{Method: "DELETE", Path: "/api/alpha"},
			{Method: "GET", Path: "/health_check"},
		},
	})

	w := httptest.NewRecorder()
	eng.GetHealth(w, newHealthRequest())

	resp := decodeBody(t, w)
	want := []Route{
		{Method: "DELETE", Path: "/api/alpha"},
		{Method: "GET", Path: "/api/alpha"},
		{Method: "POST", Path: "/api/zeta"},
		{Method: "GET", Path: "/health_check"},
	}
	if len(resp.Routes) != len(want) {
		t.Fatalf("Routes: want %d, got %d", len(want), len(resp.Routes))
	}
	for i, r := range resp.Routes {
		if r != want[i] {
			t.Errorf("Routes[%d]: want %+v, got %+v", i, want[i], r)
		}
	}
}

func TestHealth_Uptime_NonZero(t *testing.T) {
	eng := NewHealthEngine(Config{
		StartedAt: time.Now().Add(-2 * time.Second),
		DBPinger:  &fakePinger{},
	})
	w := httptest.NewRecorder()
	eng.GetHealth(w, newHealthRequest())
	resp := decodeBody(t, w)
	if resp.UptimeSeconds < 2 {
		t.Errorf("UptimeSeconds: want >=2, got %d", resp.UptimeSeconds)
	}
}

func TestParseRoutePattern_WithMethod(t *testing.T) {
	got := ParseRoutePattern("POST /api/foo")
	want := Route{Method: "POST", Path: "/api/foo"}
	if got != want {
		t.Errorf("want %+v, got %+v", want, got)
	}
}

func TestParseRoutePattern_NoMethod(t *testing.T) {
	got := ParseRoutePattern("/health_check")
	want := Route{Method: "ANY", Path: "/health_check"}
	if got != want {
		t.Errorf("want %+v, got %+v", want, got)
	}
}

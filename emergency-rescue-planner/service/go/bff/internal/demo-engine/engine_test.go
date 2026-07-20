package demo_engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHeartbeat_PersistsAutoExpand verifies the HTTP handler decodes the new
// auto_expand field from the heartbeat payload and forwards it to the store.
func TestHeartbeat_PersistsAutoExpand(t *testing.T) {
	store := NewInMemoryDemoSessionStore()
	engine := NewDemoEngine(store)

	// First heartbeat: auto_expand=true.
	body := `{"progress":0.5,"auto_expand":true}`
	req := httptest.NewRequest("POST", "/api/demo/heartbeat", strings.NewReader(body))
	req.Header.Set("X-Volunteer-Session", "sess-1")
	rr := httptest.NewRecorder()

	engine.Heartbeat(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("first heartbeat: expected 204, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	autoExpand, active, err := store.GetSessionAutoExpand(req.Context(), "sess-1")
	if err != nil {
		t.Fatalf("get auto-expand: %v", err)
	}
	if !active {
		t.Errorf("expected session active after first heartbeat")
	}
	if !autoExpand {
		t.Errorf("expected auto_expand=true persisted, got false")
	}

	// Second heartbeat: auto_expand=false (toggled off).
	body = `{"progress":0.6,"auto_expand":false}`
	req = httptest.NewRequest("POST", "/api/demo/heartbeat", strings.NewReader(body))
	req.Header.Set("X-Volunteer-Session", "sess-1")
	rr = httptest.NewRecorder()

	engine.Heartbeat(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("second heartbeat: expected 204, got %d", rr.Code)
	}
	autoExpand, _, _ = store.GetSessionAutoExpand(req.Context(), "sess-1")
	if autoExpand {
		t.Errorf("expected auto_expand=false after toggle off, got true")
	}
}

// TestHeartbeat_OmittedAutoExpand_DefaultsFalse verifies backward compat: a
// heartbeat payload without the auto_expand field still works (treated as
// false) so existing frontend code paths don't break.
func TestHeartbeat_OmittedAutoExpand_DefaultsFalse(t *testing.T) {
	store := NewInMemoryDemoSessionStore()
	engine := NewDemoEngine(store)

	body := `{"progress":0.5}` // no auto_expand field
	req := httptest.NewRequest("POST", "/api/demo/heartbeat", strings.NewReader(body))
	req.Header.Set("X-Volunteer-Session", "sess-bw-compat")
	rr := httptest.NewRecorder()

	engine.Heartbeat(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	autoExpand, active, _ := store.GetSessionAutoExpand(req.Context(), "sess-bw-compat")
	if !active {
		t.Errorf("expected session active")
	}
	if autoExpand {
		t.Errorf("expected auto_expand to default false when omitted, got true")
	}
}

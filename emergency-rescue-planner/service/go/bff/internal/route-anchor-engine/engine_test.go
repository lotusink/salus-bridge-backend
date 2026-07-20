package route_anchor_engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	conditions_engine "bff/internal/conditions-engine"
)

// =============================================================================
// Explicit per-session cleanup endpoint
// =============================================================================

// TestClearSession_RemovesSpawnedZonesAndPersons verifies that the facade's
// ClearSession delegates to OnRouteDeleted, which removes spawned zones from
// the repo and tears down scheduler session state.
func TestClearSession_RemovesSpawnedZonesAndPersons(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, zoneRepo, _ := buildFullSchedulerFixture(t, ctx, nil)
	eng := &RouteAnchorEngine{sched: sched}

	// Seed a zone owned by the session.
	zone := conditions_engine.Zone{
		ID: "z-cleanup", Level: conditions_engine.ZoneLevelHigh, Label: "test",
		Source:  "route-anchor",
		Polygon: [][2]float64{{-37.81, 144.96}, {-37.81, 144.97}, {-37.82, 144.97}, {-37.82, 144.96}},
	}
	if _, err := zoneRepo.UpsertZone(ctx, zone); err != nil {
		t.Fatalf("seed zone: %v", err)
	}

	// Inject session state pointing at the seeded zone.
	sched.mu.Lock()
	sched.sessions["sess-1"] = &RouteSchedulerSession{
		SessionID:      "sess-1",
		ActiveRouteID:  "ar-1",
		SpawnedZoneIDs: []string{"z-cleanup"},
	}
	sched.triggered["sess-1"] = map[string]bool{"h1": true}
	sched.mu.Unlock()

	eng.ClearSession(ctx, "sess-1")

	// Zone removed from repo.
	if _, err := zoneRepo.GetByID(ctx, "z-cleanup"); err != conditions_engine.ErrZoneNotFound {
		t.Errorf("expected zone deleted, got err=%v", err)
	}

	// Session + triggered map cleared.
	sched.mu.Lock()
	_, sessExists := sched.sessions["sess-1"]
	_, trigExists := sched.triggered["sess-1"]
	sched.mu.Unlock()
	if sessExists {
		t.Error("expected sessions[sess-1] removed")
	}
	if trigExists {
		t.Error("expected triggered[sess-1] removed")
	}
}

// TestClearSession_UnknownSession_NoOp: calling on an unregistered session
// must not panic, must not create phantom state.
func TestClearSession_UnknownSession_NoOp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, _, _ := buildFullSchedulerFixture(t, ctx, nil)
	eng := &RouteAnchorEngine{sched: sched}

	eng.ClearSession(ctx, "sess-never-seen") // no panic = pass

	sched.mu.Lock()
	_, exists := sched.sessions["sess-never-seen"]
	sched.mu.Unlock()
	if exists {
		t.Error("expected no phantom session entry created")
	}
}

// TestClearSessionHTTP_MissingHeader_400: missing X-Volunteer-Session header
// must return 400.
func TestClearSessionHTTP_MissingHeader_400(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, _, _ := buildFullSchedulerFixture(t, ctx, nil)
	eng := &RouteAnchorEngine{sched: sched}

	req := httptest.NewRequest("POST", "/api/demo/route-anchor/clear", nil)
	rr := httptest.NewRecorder()

	eng.ClearSessionHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// TestClearSessionHTTP_ValidSession_204: with header, returns 204 and tears
// down the session.
func TestClearSessionHTTP_ValidSession_204(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, _, _ := buildFullSchedulerFixture(t, ctx, nil)
	eng := &RouteAnchorEngine{sched: sched}

	// Pre-register a session so ClearSession has something to clear.
	sched.mu.Lock()
	sched.sessions["sess-http"] = &RouteSchedulerSession{
		SessionID:     "sess-http",
		ActiveRouteID: "ar-http",
	}
	sched.mu.Unlock()

	req := httptest.NewRequest("POST", "/api/demo/route-anchor/clear", nil)
	req.Header.Set("X-Volunteer-Session", "sess-http")
	rr := httptest.NewRecorder()

	eng.ClearSessionHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	sched.mu.Lock()
	_, exists := sched.sessions["sess-http"]
	sched.mu.Unlock()
	if exists {
		t.Error("expected session removed after ClearSessionHTTP")
	}
}

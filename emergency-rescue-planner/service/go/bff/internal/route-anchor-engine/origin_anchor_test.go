package route_anchor_engine

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	conditions_engine "bff/internal/conditions-engine"
)

// fastOriginTemplates returns test-friendly templates with millisecond-scale
// expansion so goroutine behaviour can be observed in <1s.
func fastOriginTemplates() []OriginAnchorTemplate {
	return []OriginAnchorTemplate{
		{
			ID: "cover-static", CentroidNorthM: 0, CentroidEastM: 0,
			Level: conditions_engine.ZoneLevelHigh, Label: "cover static",
			Behavior:       "static",
			PolygonOffsets: []OffsetM{{NorthM: 0, EastM: 150}, {NorthM: 150, EastM: 0}, {NorthM: 0, EastM: -150}, {NorthM: -150, EastM: 0}},
		},
		{
			ID: "cover-expand", CentroidNorthM: 0, CentroidEastM: 0,
			Level: conditions_engine.ZoneLevelHigh, Label: "cover expand",
			Behavior:              "expand",
			ExpansionTargetFactor: 2.0,
			ExpansionStepMs:       20,
			PolygonOffsets:        []OffsetM{{NorthM: 0, EastM: 100}, {NorthM: 100, EastM: 0}, {NorthM: 0, EastM: -100}, {NorthM: -100, EastM: 0}},
		},
		{
			ID: "off-static", CentroidNorthM: 500, CentroidEastM: 500,
			Level: conditions_engine.ZoneLevelHigh, Label: "off static",
			Behavior:       "static",
			PolygonOffsets: []OffsetM{{NorthM: 0, EastM: 100}, {NorthM: 100, EastM: 0}, {NorthM: 0, EastM: -100}, {NorthM: -100, EastM: 0}},
		},
		{
			ID: "off-expand", CentroidNorthM: -400, CentroidEastM: 0,
			Level: conditions_engine.ZoneLevelHigh, Label: "off expand",
			Behavior:              "expand",
			ExpansionTargetFactor: 1.8,
			ExpansionStepMs:       20,
			PolygonOffsets:        []OffsetM{{NorthM: 0, EastM: 80}, {NorthM: 80, EastM: 0}, {NorthM: 0, EastM: -80}, {NorthM: -80, EastM: 0}},
		},
	}
}

// TestSpawnOriginAnchors_CreatesAllTemplates: each call spawns one zone per
// template, all with Source="origin-anchor" and tracked in
// SpawnedOriginAnchorIDs.
func TestSpawnOriginAnchors_CreatesAllTemplates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, zoneRepo, _ := buildFullSchedulerFixture(t, ctx, nil)

	origin := LatLng{Lat: -37.81, Lng: 144.96}
	sched.spawnOriginAnchorsImpl(ctx, "sess-1", origin, true, fastOriginTemplates())

	// Count origin-anchor zones in repo.
	all, _ := zoneRepo.GetAll(ctx)
	oaCount := 0
	for _, z := range all {
		if z.Source == "origin-anchor" {
			oaCount++
		}
	}
	if oaCount != 4 {
		t.Errorf("expected 4 origin-anchor zones in repo, got %d", oaCount)
	}

	// SpawnedOriginAnchorIDs has 4 entries.
	sched.mu.Lock()
	sess := sched.sessions["sess-1"]
	spawnedLen := 0
	if sess != nil {
		spawnedLen = len(sess.SpawnedOriginAnchorIDs)
	}
	sched.mu.Unlock()
	if spawnedLen != 4 {
		t.Errorf("expected 4 SpawnedOriginAnchorIDs, got %d", spawnedLen)
	}
}

// TestClearOriginAnchors_RemovesOnlyOriginAnchorZones: ClearOriginAnchors
// must leave SpawnedZoneIDs (route-anchor) untouched.
func TestClearOriginAnchors_RemovesOnlyOriginAnchorZones(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, zoneRepo, _ := buildFullSchedulerFixture(t, ctx, nil)

	origin := LatLng{Lat: -37.81, Lng: 144.96}
	sched.spawnOriginAnchorsImpl(ctx, "sess-1", origin, true, fastOriginTemplates())

	// Add a route-anchor zone separately under the same session.
	routeAnchorID := "rha-sess-1-fake"
	if _, err := zoneRepo.UpsertZone(ctx, conditions_engine.Zone{
		ID: routeAnchorID, Level: conditions_engine.ZoneLevelHigh, Label: "route",
		Source:  "route-anchor",
		Polygon: [][2]float64{{-37.81, 144.96}, {-37.81, 144.97}, {-37.82, 144.97}, {-37.82, 144.96}},
	}); err != nil {
		t.Fatalf("seed route-anchor zone: %v", err)
	}
	sched.mu.Lock()
	sched.sessions["sess-1"].SpawnedZoneIDs = []string{routeAnchorID}
	sched.mu.Unlock()

	sched.ClearOriginAnchors(ctx, "sess-1")

	// All 4 origin-anchor zones gone.
	all, _ := zoneRepo.GetAll(ctx)
	for _, z := range all {
		if z.Source == "origin-anchor" {
			t.Errorf("origin-anchor zone %s should be deleted", z.ID)
		}
	}

	// Route-anchor zone still present.
	if _, err := zoneRepo.GetByID(ctx, routeAnchorID); err != nil {
		t.Errorf("route-anchor zone should remain, got err: %v", err)
	}

	// Session state: SpawnedOriginAnchorIDs cleared, SpawnedZoneIDs intact.
	sched.mu.Lock()
	sess := sched.sessions["sess-1"]
	oaLen := len(sess.SpawnedOriginAnchorIDs)
	raLen := len(sess.SpawnedZoneIDs)
	sched.mu.Unlock()
	if oaLen != 0 {
		t.Errorf("expected 0 SpawnedOriginAnchorIDs, got %d", oaLen)
	}
	if raLen != 1 {
		t.Errorf("expected 1 SpawnedZoneIDs preserved, got %d", raLen)
	}
}

// TestSpawnOriginAnchors_Idempotent: calling twice replaces (not accumulates)
// the spawned zones for that session.
func TestSpawnOriginAnchors_Idempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, zoneRepo, _ := buildFullSchedulerFixture(t, ctx, nil)

	origin := LatLng{Lat: -37.81, Lng: 144.96}
	templates := fastOriginTemplates()
	sched.spawnOriginAnchorsImpl(ctx, "sess-1", origin, true, templates)
	sched.spawnOriginAnchorsImpl(ctx, "sess-1", origin, true, templates)

	all, _ := zoneRepo.GetAll(ctx)
	oaCount := 0
	for _, z := range all {
		if z.Source == "origin-anchor" {
			oaCount++
		}
	}
	if oaCount != 4 {
		t.Errorf("expected exactly 4 origin-anchor zones after 2 spawn calls, got %d", oaCount)
	}
}

// TestOriginExpansionGoroutine_GrowsPolygon: the expand template's polygon
// grows over time. Verify the final polygon is larger than baseline.
func TestOriginExpansionGoroutine_GrowsPolygon(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, zoneRepo, _ := buildFullSchedulerFixture(t, ctx, nil)

	origin := LatLng{Lat: -37.81, Lng: 144.96}
	// Single template, expand to 2x in 10 steps × 20ms ≈ 200ms.
	templates := []OriginAnchorTemplate{{
		ID: "test-grow", CentroidNorthM: 0, CentroidEastM: 0,
		Level: conditions_engine.ZoneLevelHigh, Label: "grow",
		Behavior:              "expand",
		ExpansionTargetFactor: 2.0,
		ExpansionStepMs:       20,
		PolygonOffsets:        []OffsetM{{NorthM: 0, EastM: 100}, {NorthM: 100, EastM: 0}, {NorthM: 0, EastM: -100}, {NorthM: -100, EastM: 0}},
	}}
	sched.spawnOriginAnchorsImpl(ctx, "sess-1", origin, true, templates)

	// Initial polygon: ~100m offsets from centroid.
	zoneID := "oa-sess-1-test-grow"
	initial, err := zoneRepo.GetByID(ctx, zoneID)
	if err != nil {
		t.Fatalf("zone not found post-spawn: %v", err)
	}
	if len(initial.Polygon) == 0 {
		t.Fatal("initial polygon empty")
	}

	// Wait for expansion to complete (10 × 20ms = 200ms + buffer).
	time.Sleep(350 * time.Millisecond)

	final, err := zoneRepo.GetByID(ctx, zoneID)
	if err != nil {
		t.Fatalf("zone vanished mid-test: %v", err)
	}
	// Final polygon should be larger: vertex spread from centroid is roughly
	// 2x of initial. Pick the easternmost vertex (index 0 in our offset list).
	// Approx: 2x * 100m ≈ 200m east of origin → 0.001m longitude bigger than
	// initial at this latitude. Just check that maxLng grew.
	initialMaxLng := initial.Polygon[0][1]
	for _, p := range initial.Polygon {
		if p[1] > initialMaxLng {
			initialMaxLng = p[1]
		}
	}
	finalMaxLng := final.Polygon[0][1]
	for _, p := range final.Polygon {
		if p[1] > finalMaxLng {
			finalMaxLng = p[1]
		}
	}
	if finalMaxLng <= initialMaxLng+0.0005 {
		t.Errorf("expected polygon to expand: initialMaxLng=%v finalMaxLng=%v", initialMaxLng, finalMaxLng)
	}
}

// TestOriginExpansionGoroutine_ContextCancel: cancelling the scheduler ctx
// exits the expansion goroutine cleanly. race detector catches leaks.
func TestOriginExpansionGoroutine_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	sched, _, _ := buildFullSchedulerFixture(t, ctx, nil)
	origin := LatLng{Lat: -37.81, Lng: 144.96}
	templates := []OriginAnchorTemplate{{
		ID: "long-exp", CentroidNorthM: 0, CentroidEastM: 0,
		Level: conditions_engine.ZoneLevelHigh, Label: "long",
		Behavior:              "expand",
		ExpansionTargetFactor: 5.0,
		ExpansionStepMs:       50,
		PolygonOffsets:        []OffsetM{{NorthM: 0, EastM: 50}, {NorthM: 50, EastM: 0}, {NorthM: 0, EastM: -50}, {NorthM: -50, EastM: 0}},
	}}
	sched.spawnOriginAnchorsImpl(ctx, "sess-1", origin, true, templates)
	time.Sleep(80 * time.Millisecond) // let one step fire
	cancel()                          // signal stop
	time.Sleep(150 * time.Millisecond)
	// no panic, no race = pass
}

// TestSpawnOriginAnchorsHTTP_MissingSession_400: missing X-Volunteer-Session
// header → 400.
func TestSpawnOriginAnchorsHTTP_MissingSession_400(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, _, _ := buildFullSchedulerFixture(t, ctx, nil)
	eng := &RouteAnchorEngine{sched: sched}

	body := `{"origin":{"lat":-37.81,"lng":144.96}}`
	req := httptest.NewRequest("POST", "/api/demo/origin-anchor/spawn", strings.NewReader(body))
	rr := httptest.NewRecorder()

	eng.SpawnOriginAnchorsHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// TestSpawnOriginAnchorsHTTP_ValidBody_204: valid header + body → 204 +
// zones spawned.
func TestSpawnOriginAnchorsHTTP_ValidBody_204(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, zoneRepo, _ := buildFullSchedulerFixture(t, ctx, nil)
	eng := &RouteAnchorEngine{sched: sched}

	body := `{"origin":{"lat":-37.81,"lng":144.96}}`
	req := httptest.NewRequest("POST", "/api/demo/origin-anchor/spawn", strings.NewReader(body))
	req.Header.Set("X-Volunteer-Session", "sess-http")
	rr := httptest.NewRecorder()

	eng.SpawnOriginAnchorsHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	all, _ := zoneRepo.GetAll(ctx)
	oaCount := 0
	for _, z := range all {
		if z.Source == "origin-anchor" {
			oaCount++
		}
	}
	if oaCount != 4 {
		t.Errorf("expected 4 origin-anchor zones after HTTP spawn, got %d", oaCount)
	}
}

// =============================================================================
// Auto-expand gate for origin-anchor expansion
// =============================================================================

// TestSpawnOriginAnchors_StoresAutoExpand verifies that the autoExpand
// argument is persisted on the session for runtime lookup.
func TestSpawnOriginAnchors_StoresAutoExpand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, _, _ := buildFullSchedulerFixture(t, ctx, nil)

	origin := LatLng{Lat: -37.81, Lng: 144.96}
	sched.spawnOriginAnchorsImpl(ctx, "sess-1", origin, true, fastOriginTemplates())

	sched.mu.Lock()
	got := sched.sessions["sess-1"].OriginAnchorAutoExpand
	sched.mu.Unlock()
	if !got {
		t.Errorf("expected OriginAnchorAutoExpand=true after spawn, got false")
	}
}

// TestOriginExpansion_AutoExpandOff_NoGrowth: spawn an expand template with
// autoExpand=false; verify polygon stays at baseline after several tick
// intervals (factor never advances).
func TestOriginExpansion_AutoExpandOff_NoGrowth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, zoneRepo, _ := buildFullSchedulerFixture(t, ctx, nil)

	origin := LatLng{Lat: -37.81, Lng: 144.96}
	templates := []OriginAnchorTemplate{{
		ID: "test-gated", CentroidNorthM: 0, CentroidEastM: 0,
		Level: conditions_engine.ZoneLevelHigh, Label: "gated",
		Behavior:              "expand",
		ExpansionTargetFactor: 2.0,
		ExpansionStepMs:       20,
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 100}, {NorthM: 100, EastM: 0},
			{NorthM: 0, EastM: -100}, {NorthM: -100, EastM: 0},
		},
	}}
	sched.spawnOriginAnchorsImpl(ctx, "sess-1", origin, false, templates)

	zoneID := "oa-sess-1-test-gated"
	initial, err := zoneRepo.GetByID(ctx, zoneID)
	if err != nil {
		t.Fatalf("zone not spawned: %v", err)
	}

	// Wait significantly longer than 10 step intervals.
	time.Sleep(400 * time.Millisecond)

	final, err := zoneRepo.GetByID(ctx, zoneID)
	if err != nil {
		t.Fatalf("zone vanished: %v", err)
	}

	// Compare maxLng — if expansion happened, it would grow visibly.
	maxLng := func(poly [][2]float64) float64 {
		m := poly[0][1]
		for _, p := range poly {
			if p[1] > m {
				m = p[1]
			}
		}
		return m
	}
	initialMax := maxLng(initial.Polygon)
	finalMax := maxLng(final.Polygon)
	if math.Abs(finalMax-initialMax) > 1e-9 {
		t.Errorf("polygon should not have grown (autoExpand=false): initialMaxLng=%v finalMaxLng=%v", initialMax, finalMax)
	}
}

// TestOriginExpansion_ResumesOnAutoExpandToggle: spawn with autoExpand=false,
// then SetOriginAnchorAutoExpand(true) → polygon grows from baseline. Verifies
// the gate is consulted each tick (not only at spawn time).
func TestOriginExpansion_ResumesOnAutoExpandToggle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, zoneRepo, _ := buildFullSchedulerFixture(t, ctx, nil)

	origin := LatLng{Lat: -37.81, Lng: 144.96}
	templates := []OriginAnchorTemplate{{
		ID: "test-toggle", CentroidNorthM: 0, CentroidEastM: 0,
		Level: conditions_engine.ZoneLevelHigh, Label: "toggle",
		Behavior:              "expand",
		ExpansionTargetFactor: 2.0,
		ExpansionStepMs:       20,
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 100}, {NorthM: 100, EastM: 0},
			{NorthM: 0, EastM: -100}, {NorthM: -100, EastM: 0},
		},
	}}
	sched.spawnOriginAnchorsImpl(ctx, "sess-1", origin, false, templates)

	zoneID := "oa-sess-1-test-toggle"
	initial, _ := zoneRepo.GetByID(ctx, zoneID)
	maxLng := func(poly [][2]float64) float64 {
		m := poly[0][1]
		for _, p := range poly {
			if p[1] > m {
				m = p[1]
			}
		}
		return m
	}
	initialMax := maxLng(initial.Polygon)

	// Confirm no growth before toggle.
	time.Sleep(80 * time.Millisecond)
	mid, _ := zoneRepo.GetByID(ctx, zoneID)
	if math.Abs(maxLng(mid.Polygon)-initialMax) > 1e-9 {
		t.Fatalf("polygon grew before toggle (autoExpand=false): %v vs %v", maxLng(mid.Polygon), initialMax)
	}

	// Toggle on — goroutine should resume on next tick.
	sched.SetOriginAnchorAutoExpand("sess-1", true)
	time.Sleep(350 * time.Millisecond)

	final, _ := zoneRepo.GetByID(ctx, zoneID)
	finalMax := maxLng(final.Polygon)
	if finalMax <= initialMax+0.0005 {
		t.Errorf("polygon did not grow after autoExpand toggle: initial=%v final=%v", initialMax, finalMax)
	}
}

// TestSetOriginAnchorAutoExpandHTTP_MissingSession_400: missing
// X-Volunteer-Session → 400.
func TestSetOriginAnchorAutoExpandHTTP_MissingSession_400(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, _, _ := buildFullSchedulerFixture(t, ctx, nil)
	eng := &RouteAnchorEngine{sched: sched}

	body := `{"enabled":true}`
	req := httptest.NewRequest("POST", "/api/demo/origin-anchor/auto-expand", strings.NewReader(body))
	rr := httptest.NewRecorder()

	eng.SetOriginAnchorAutoExpandHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// TestSetOriginAnchorAutoExpandHTTP_ValidBody_204: valid request flips
// the session's auto-expand flag.
func TestSetOriginAnchorAutoExpandHTTP_ValidBody_204(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched, _, _ := buildFullSchedulerFixture(t, ctx, nil)
	eng := &RouteAnchorEngine{sched: sched}

	// Pre-populate session so SetOriginAnchorAutoExpand has somewhere to write.
	sched.mu.Lock()
	sched.sessions["sess-1"] = &RouteSchedulerSession{SessionID: "sess-1"}
	sched.mu.Unlock()

	body := `{"enabled":true}`
	req := httptest.NewRequest("POST", "/api/demo/origin-anchor/auto-expand", strings.NewReader(body))
	req.Header.Set("X-Volunteer-Session", "sess-1")
	rr := httptest.NewRecorder()

	eng.SetOriginAnchorAutoExpandHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	sched.mu.Lock()
	got := sched.sessions["sess-1"].OriginAnchorAutoExpand
	sched.mu.Unlock()
	if !got {
		t.Errorf("expected OriginAnchorAutoExpand=true after HTTP toggle")
	}
}

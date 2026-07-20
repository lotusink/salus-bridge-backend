package route_anchor_engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	active_route_engine "bff/internal/active-route-engine"
	conditions_engine "bff/internal/conditions-engine"
	demo_engine "bff/internal/demo-engine"
	persons_engine "bff/internal/persons-engine"
	route_engine "bff/internal/route-engine"
)

// newTestScheduler builds a RouteScheduler with only the fields OnRerouteAccepted
// touches. All I/O collaborators (zoneRepo, cEngine, hub, etc.) are left nil
// because OnRerouteAccepted is a pure state mutation; it must not invoke them.
func newTestScheduler() *RouteScheduler {
	return &RouteScheduler{
		sessions:  make(map[string]*RouteSchedulerSession),
		triggered: make(map[string]map[string]bool),
		hazardScript: []HazardAnchor{
			{ID: "h1", TriggerAtProgress: 0.10, CentroidAtProgress: 0.20},
			{ID: "h2", TriggerAtProgress: 0.40, CentroidAtProgress: 0.50},
		},
		ctx: context.Background(),
	}
}

// TestOnRerouteAccepted_PreservesTriggeredMap verifies that anchors already
// fired on the pre-reroute polyline stay in the triggered set after reroute.
// Rationale: user already saw / passed those hazards; refiring them on the
// new polyline would be a UX bug.
func TestOnRerouteAccepted_PreservesTriggeredMap(t *testing.T) {
	s := newTestScheduler()
	s.sessions["sess-1"] = &RouteSchedulerSession{
		SessionID:     "sess-1",
		ActiveRouteID: "ar-1",
		Polyline:      []LatLng{{Lat: -37.81, Lng: 144.96}, {Lat: -37.82, Lng: 144.97}},
		NextHazardIdx: 1,
	}
	s.triggered["sess-1"] = map[string]bool{"h1": true}

	newPolyline := []LatLng{{Lat: -37.85, Lng: 144.99}, {Lat: -37.86, Lng: 145.00}}
	s.OnRerouteAccepted(context.Background(), "sess-1", newPolyline)

	if !s.triggered["sess-1"]["h1"] {
		t.Error("expected h1 to remain in triggered map after reroute; got missing/false")
	}
	if len(s.triggered["sess-1"]) != 1 {
		t.Errorf("expected triggered map size 1, got %d", len(s.triggered["sess-1"]))
	}
}

// TestOnRerouteAccepted_RescansForUntriggered verifies that NextHazardIdx is
// reset to 0 so post-reroute heartbeats walk the full script. Already-triggered
// anchors will be skipped by the idempotency check in OnHeartbeat; not-yet-
// triggered anchors get a fresh chance to fire at their progress threshold on
// the new polyline.
func TestOnRerouteAccepted_RescansForUntriggered(t *testing.T) {
	s := newTestScheduler()
	s.sessions["sess-1"] = &RouteSchedulerSession{
		SessionID:     "sess-1",
		ActiveRouteID: "ar-1",
		Polyline:      []LatLng{{Lat: -37.81, Lng: 144.96}},
		NextHazardIdx: 2, // walked past all anchors on the old polyline
		NextPersonIdx: 3,
	}

	newPolyline := []LatLng{{Lat: -37.85, Lng: 144.99}}
	s.OnRerouteAccepted(context.Background(), "sess-1", newPolyline)

	if got := s.sessions["sess-1"].NextHazardIdx; got != 0 {
		t.Errorf("expected NextHazardIdx reset to 0, got %d", got)
	}
	if got := s.sessions["sess-1"].NextPersonIdx; got != 0 {
		t.Errorf("expected NextPersonIdx reset to 0, got %d", got)
	}
}

// TestOnRerouteAccepted_UpdatesPolyline verifies that the session's Polyline
// is replaced with the new polyline so subsequent anchor projections use the
// correct geometry.
func TestOnRerouteAccepted_UpdatesPolyline(t *testing.T) {
	s := newTestScheduler()
	oldPoly := []LatLng{{Lat: -37.81, Lng: 144.96}, {Lat: -37.82, Lng: 144.97}}
	s.sessions["sess-1"] = &RouteSchedulerSession{
		SessionID:     "sess-1",
		ActiveRouteID: "ar-1",
		Polyline:      oldPoly,
	}

	newPoly := []LatLng{
		{Lat: -37.85, Lng: 144.99},
		{Lat: -37.86, Lng: 145.00},
		{Lat: -37.87, Lng: 145.01},
	}
	s.OnRerouteAccepted(context.Background(), "sess-1", newPoly)

	got := s.sessions["sess-1"].Polyline
	if len(got) != 3 {
		t.Fatalf("expected new polyline length 3, got %d", len(got))
	}
	if got[0].Lat != -37.85 || got[0].Lng != 144.99 {
		t.Errorf("expected first vertex (-37.85, 144.99), got (%v, %v)", got[0].Lat, got[0].Lng)
	}
	if got[2].Lat != -37.87 || got[2].Lng != 145.01 {
		t.Errorf("expected last vertex (-37.87, 145.01), got (%v, %v)", got[2].Lat, got[2].Lng)
	}
}

// TestOnRerouteAccepted_UnknownSession_Noop verifies that calling the hook for
// a session that the scheduler has never seen does not panic and does not
// create a phantom session entry. AcceptReroute can be called for routes that
// have not yet received a heartbeat; the scheduler must tolerate this.
func TestOnRerouteAccepted_UnknownSession_Noop(t *testing.T) {
	s := newTestScheduler()
	newPoly := []LatLng{{Lat: -37.85, Lng: 144.99}}

	// Should not panic.
	s.OnRerouteAccepted(context.Background(), "sess-unknown", newPoly)

	if _, exists := s.sessions["sess-unknown"]; exists {
		t.Error("expected no session created for unknown ID; scheduler is heartbeat-driven for session creation")
	}
}

// =============================================================================
// δ lifecycle tests
// =============================================================================

// buildFullSchedulerFixture wires up a real RouteScheduler with in-memory mocks
// of every collaborator. Used by lifecycle integration tests that exercise the
// full anchor-fire → zone-spawn → zone-clear chain. No HTTP, no WS clients.
func buildFullSchedulerFixture(t *testing.T, ctx context.Context, hazardScript []HazardAnchor) (
	*RouteScheduler,
	conditions_engine.ZoneRepository,
	active_route_engine.ActiveRouteRepository,
) {
	t.Helper()
	zoneRepo := conditions_engine.NewMockZoneRepository()
	cEngine := conditions_engine.NewConditionsEngine(zoneRepo)
	cEngine.StartHub(ctx) // mirror main.go:97 — Hub() returns nil until this is called
	arRepo := active_route_engine.NewMockActiveRouteRepository()
	personRepo := persons_engine.NewMockPersonRepository()
	demoStore := demo_engine.NewInMemoryDemoSessionStore()
	sched := newRouteScheduler(
		hazardScript, nil, ctx, zoneRepo, cEngine, cEngine.Hub(),
		arRepo, personRepo, demoStore,
	)
	return sched, zoneRepo, arRepo
}

// TestDeleteSpawnedZone_RemovesFromRepoAndSession is a unit test of the
// scheduler-internal helper that backs both the route-deletion sweep and the
// δ lifecycle goroutine. It must remove the zone id from
// SpawnedZoneIDs *and* delete the underlying repository entry.
func TestDeleteSpawnedZone_RemovesFromRepoAndSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, zoneRepo, _ := buildFullSchedulerFixture(t, ctx, nil)

	// Seed: zone in repo + session that owns it.
	zone := conditions_engine.Zone{
		ID: "z-1", Level: conditions_engine.ZoneLevelHigh, Label: "test", Source: "route-anchor",
		Polygon: [][2]float64{{-37.81, 144.96}, {-37.81, 144.97}, {-37.82, 144.97}, {-37.82, 144.96}},
	}
	if _, err := zoneRepo.UpsertZone(ctx, zone); err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	s.mu.Lock()
	s.sessions["sess-1"] = &RouteSchedulerSession{
		SessionID: "sess-1", ActiveRouteID: "ar-1",
		SpawnedZoneIDs: []string{"z-other-1", "z-1", "z-other-2"},
	}
	s.mu.Unlock()

	s.deleteSpawnedZone("sess-1", "z-1")

	// Zone should be gone from repo.
	if _, err := zoneRepo.GetByID(ctx, "z-1"); err != conditions_engine.ErrZoneNotFound {
		t.Errorf("expected ErrZoneNotFound after delete, got %v", err)
	}
	// Session's SpawnedZoneIDs should no longer contain z-1, but the other two stay.
	s.mu.Lock()
	got := append([]string(nil), s.sessions["sess-1"].SpawnedZoneIDs...)
	s.mu.Unlock()
	if len(got) != 2 || got[0] != "z-other-1" || got[1] != "z-other-2" {
		t.Errorf("expected SpawnedZoneIDs=[z-other-1, z-other-2], got %v", got)
	}
}

// =============================================================================
// β expansion tests
// =============================================================================

// betaTestRoute returns a simple west-east polyline near Melbourne CBD used as
// the active route in β fixtures.
func betaTestRoute() active_route_engine.ActiveRoute {
	return active_route_engine.ActiveRoute{
		VolunteerSession: "sess-1",
		Origin:           active_route_engine.LatLng{Lat: -37.81, Lng: 144.96},
		Destination:      active_route_engine.LatLng{Lat: -37.81, Lng: 144.99},
		Geometry: route_engine.GeoJSONGeometry{
			Type: "LineString",
			Coordinates: [][]float64{
				{144.96, -37.81}, {144.97, -37.81}, {144.98, -37.81}, {144.99, -37.81},
			},
		},
	}
}

// TestBeta_AutoExpandOff_NoExpansion: β anchor fires while session has
// auto_expand=false. Zone spawns at baseline; no expansion goroutine kicks
// in, no UpdateZone broadcasts happen.
func TestBeta_AutoExpandOff_NoExpansion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hazardScript := []HazardAnchor{{
		ID: "h-beta", TriggerAtProgress: 0.1, CentroidAtProgress: 0.5,
		PerpendicularOffsetM: 800, // off-route start
		Level:                conditions_engine.ZoneLevelHigh,
		Label:                "β no-expand", Source: "route-anchor",
		Behavior:              "expand",
		ExpansionTargetFactor: 2.0,
		ExpansionStepMs:       40,
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 50}, {NorthM: 50, EastM: 0},
			{NorthM: 0, EastM: -50}, {NorthM: -50, EastM: 0},
		},
	}}
	s, zoneRepo, arRepo := buildFullSchedulerFixture(t, ctx, hazardScript)

	if _, err := arRepo.Register(ctx, betaTestRoute()); err != nil {
		t.Fatalf("register route: %v", err)
	}
	// Mark session as demo-active with auto_expand=false.
	if err := s.demoStore.Upsert(ctx, "sess-1", 0.0, false); err != nil {
		t.Fatalf("seed demo store: %v", err)
	}

	s.OnHeartbeat("sess-1", 0.5)
	zoneID := "rha-sess-1-h-beta"
	zone, err := zoneRepo.GetByID(ctx, zoneID)
	if err != nil {
		t.Fatalf("expected zone spawned, got %v", err)
	}
	originalPoly := append([][2]float64(nil), zone.Polygon...)

	// Wait several expansion steps' worth of time. If expansion goroutine were
	// running, polygon would have grown.
	time.Sleep(200 * time.Millisecond)

	current, err := zoneRepo.GetByID(ctx, zoneID)
	if err != nil {
		t.Fatalf("zone vanished mid-test: %v", err)
	}
	if !polygonsEqual(originalPoly, current.Polygon) {
		t.Errorf("polygon changed despite auto_expand=false: before=%v after=%v", originalPoly, current.Polygon)
	}
}

// TestBeta_AutoExpandOn_ExpandsUntilIntersect: β anchor fires with auto_expand
// on; the expansion goroutine grows polygon until it intersects the route,
// then fires the route-recompute hook exactly once and stops.
func TestBeta_AutoExpandOn_ExpandsUntilIntersect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hazardScript := []HazardAnchor{{
		ID: "h-beta", TriggerAtProgress: 0.1, CentroidAtProgress: 0.5,
		PerpendicularOffsetM: 200, // off-route, but close enough to hit on 2-3 steps
		Level:                conditions_engine.ZoneLevelHigh,
		Label:                "β expand-to-intersect", Source: "route-anchor",
		Behavior:              "expand",
		ExpansionTargetFactor: 3.0, // generous so intersect comes first
		ExpansionStepMs:       40,
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 80}, {NorthM: 80, EastM: 0},
			{NorthM: 0, EastM: -80}, {NorthM: -80, EastM: 0},
		},
	}}
	s, _, arRepo := buildFullSchedulerFixture(t, ctx, hazardScript)

	if _, err := arRepo.Register(ctx, betaTestRoute()); err != nil {
		t.Fatalf("register route: %v", err)
	}
	if err := s.demoStore.Upsert(ctx, "sess-1", 0.0, true); err != nil {
		t.Fatalf("seed demo store: %v", err)
	}

	// Inject a recompute hook that records calls (atomic — read by test main
	// goroutine, written by runExpansion goroutine).
	var hookFiredCount atomic.Int32
	s.routeRecomputeHook = func(_ context.Context, _ conditions_engine.Zone) {
		hookFiredCount.Add(1)
	}

	s.OnHeartbeat("sess-1", 0.5)

	// Wait long enough for expansion to find intersect; capped to avoid hanging
	// indefinitely on a test bug.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && hookFiredCount.Load() == 0 {
		time.Sleep(30 * time.Millisecond)
	}

	if got := hookFiredCount.Load(); got != 1 {
		t.Errorf("expected exactly 1 routeRecomputeHook fire, got %d", got)
	}

	// After the hook fires, expansion should be stopped. No further hook calls
	// during a generous tail window.
	time.Sleep(200 * time.Millisecond)
	if got := hookFiredCount.Load(); got > 1 {
		t.Errorf("expected expansion to stop after intersect; got %d total hook fires", got)
	}
}

// TestBeta_AutoExpandOn_ReachesMaxNoIntersect: β anchor at large perp offset;
// expansion grows to ExpansionTargetFactor but never intersects route → no
// recompute hook fired, goroutine exits cleanly.
func TestBeta_AutoExpandOn_ReachesMaxNoIntersect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hazardScript := []HazardAnchor{{
		ID: "h-beta", TriggerAtProgress: 0.1, CentroidAtProgress: 0.5,
		PerpendicularOffsetM: 5000, // very far — even 2x expansion won't reach route
		Level:                conditions_engine.ZoneLevelHigh,
		Label:                "β reach-max", Source: "route-anchor",
		Behavior:              "expand",
		ExpansionTargetFactor: 2.0,
		ExpansionStepMs:       30,
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 50}, {NorthM: 50, EastM: 0},
			{NorthM: 0, EastM: -50}, {NorthM: -50, EastM: 0},
		},
	}}
	s, _, arRepo := buildFullSchedulerFixture(t, ctx, hazardScript)

	if _, err := arRepo.Register(ctx, betaTestRoute()); err != nil {
		t.Fatalf("register route: %v", err)
	}
	if err := s.demoStore.Upsert(ctx, "sess-1", 0.0, true); err != nil {
		t.Fatalf("seed demo store: %v", err)
	}
	var hookFired atomic.Int32
	s.routeRecomputeHook = func(_ context.Context, _ conditions_engine.Zone) {
		hookFired.Add(1)
	}

	s.OnHeartbeat("sess-1", 0.5)
	// Expansion of 10 steps × 30 ms ≈ 300 ms; wait 600 ms to give it more
	// than enough time.
	time.Sleep(600 * time.Millisecond)

	if got := hookFired.Load(); got != 0 {
		t.Errorf("expected no recompute hook fires (target reached without intersect), got %d", got)
	}
}

// TestBeta_ExpansionGoroutine_ContextCancel: cancelling the scheduler ctx
// during active expansion exits the goroutine promptly with no further
// UpdateZone calls.
func TestBeta_ExpansionGoroutine_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	hazardScript := []HazardAnchor{{
		ID: "h-beta", TriggerAtProgress: 0.1, CentroidAtProgress: 0.5,
		PerpendicularOffsetM: 3000,
		Level:                conditions_engine.ZoneLevelHigh,
		Label:                "β ctx-cancel", Source: "route-anchor",
		Behavior:              "expand",
		ExpansionTargetFactor: 5.0, // make expansion long enough to be cancelled mid-flight
		ExpansionStepMs:       50,
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 50}, {NorthM: 50, EastM: 0},
			{NorthM: 0, EastM: -50}, {NorthM: -50, EastM: 0},
		},
	}}
	s, _, arRepo := buildFullSchedulerFixture(t, ctx, hazardScript)

	if _, err := arRepo.Register(ctx, betaTestRoute()); err != nil {
		t.Fatalf("register route: %v", err)
	}
	if err := s.demoStore.Upsert(ctx, "sess-1", 0.0, true); err != nil {
		t.Fatalf("seed demo store: %v", err)
	}

	s.OnHeartbeat("sess-1", 0.5)
	time.Sleep(70 * time.Millisecond) // let one or two steps run
	cancel()                          // cancel context

	// After cancel, goroutine should exit within one step duration.
	// We can't directly detect goroutine exit, but the race detector and
	// goroutine leak detector in -race mode will catch leaks. Best we can
	// do here is sleep + verify no panic.
	time.Sleep(150 * time.Millisecond)
	// Implicit: no panic, no race detected by go test -race.
}

// TestBeta_HeartbeatTogglesAutoExpand: heartbeat that crosses anchor trigger
// also carries the auto_expand flag — fresh read of session at fire time
// drives the expansion decision (no stale read).
func TestBeta_HeartbeatTogglesAutoExpand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hazardScript := []HazardAnchor{{
		ID: "h-beta", TriggerAtProgress: 0.4, CentroidAtProgress: 0.5,
		PerpendicularOffsetM: 200,
		Level:                conditions_engine.ZoneLevelHigh,
		Label:                "β toggle", Source: "route-anchor",
		Behavior:              "expand",
		ExpansionTargetFactor: 3.0,
		ExpansionStepMs:       40,
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 80}, {NorthM: 80, EastM: 0},
			{NorthM: 0, EastM: -80}, {NorthM: -80, EastM: 0},
		},
	}}
	s, _, arRepo := buildFullSchedulerFixture(t, ctx, hazardScript)

	if _, err := arRepo.Register(ctx, betaTestRoute()); err != nil {
		t.Fatalf("register route: %v", err)
	}
	var hookFired atomic.Int32
	s.routeRecomputeHook = func(_ context.Context, _ conditions_engine.Zone) {
		hookFired.Add(1)
	}

	// Pre-trigger: auto_expand=false, progress 0.1 (below anchor threshold).
	_ = s.demoStore.Upsert(ctx, "sess-1", 0.1, false)
	s.OnHeartbeat("sess-1", 0.1)
	// No anchor fire yet.

	// Now bump auto_expand=true and cross the threshold in one go.
	_ = s.demoStore.Upsert(ctx, "sess-1", 0.5, true)
	s.OnHeartbeat("sess-1", 0.5)

	// Expansion should now kick in and eventually hit the route.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && hookFired.Load() == 0 {
		time.Sleep(30 * time.Millisecond)
	}
	if got := hookFired.Load(); got != 1 {
		t.Errorf("expected expansion to trigger after auto_expand=true heartbeat, got %d hook fires", got)
	}
}

// polygonsEqual compares two polygon vertex slices with a small float
// tolerance.
func polygonsEqual(a, b [][2]float64) bool {
	if len(a) != len(b) {
		return false
	}
	const eps = 1e-9
	for i := range a {
		if absF(a[i][0]-b[i][0]) > eps || absF(a[i][1]-b[i][1]) > eps {
			return false
		}
	}
	return true
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// TestAnchorFire_LifecycleMs_TriggersDeleteAfterDelay verifies the δ
// behaviour: anchor with LifecycleMs > 0 spawns a zone, then the zone is
// auto-deactivated after that delay without further heartbeats.
func TestAnchorFire_LifecycleMs_TriggersDeleteAfterDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const lifecycle = 60 * time.Millisecond
	hazardScript := []HazardAnchor{{
		ID: "h-delta", TriggerAtProgress: 0.1, CentroidAtProgress: 0.2,
		PerpendicularOffsetM: 0,
		Level:                conditions_engine.ZoneLevelHigh,
		Label:                "δ test", Source: "route-anchor",
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 50}, {NorthM: 50, EastM: 0},
			{NorthM: 0, EastM: -50}, {NorthM: -50, EastM: 0},
		},
		LifecycleMs: int(lifecycle / time.Millisecond),
	}}
	s, zoneRepo, arRepo := buildFullSchedulerFixture(t, ctx, hazardScript)

	// Register an active route so OnHeartbeat can lazy-init the session.
	route := active_route_engine.ActiveRoute{
		VolunteerSession: "sess-1",
		Origin:           active_route_engine.LatLng{Lat: -37.81, Lng: 144.96},
		Destination:      active_route_engine.LatLng{Lat: -37.82, Lng: 144.97},
		Geometry: route_engine.GeoJSONGeometry{
			Type:        "LineString",
			Coordinates: [][]float64{{144.96, -37.81}, {144.97, -37.82}},
		},
	}
	if _, err := arRepo.Register(ctx, route); err != nil {
		t.Fatalf("register route: %v", err)
	}

	// Cross the anchor's trigger threshold.
	s.OnHeartbeat("sess-1", 0.5)

	// Zone should now exist.
	zoneID := "rha-sess-1-h-delta"
	if _, err := zoneRepo.GetByID(ctx, zoneID); err != nil {
		t.Fatalf("expected zone %s to exist post-fire, got %v", zoneID, err)
	}

	// Wait past LifecycleMs + a generous buffer.
	time.Sleep(lifecycle + 100*time.Millisecond)

	if _, err := zoneRepo.GetByID(ctx, zoneID); err != conditions_engine.ErrZoneNotFound {
		t.Errorf("expected zone %s to be deleted after lifecycle elapsed, got %v", zoneID, err)
	}
}

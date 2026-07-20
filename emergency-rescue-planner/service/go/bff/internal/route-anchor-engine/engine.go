package route_anchor_engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	active_route_engine "bff/internal/active-route-engine"
	conditions_engine "bff/internal/conditions-engine"
	demo_engine "bff/internal/demo-engine"
	persons_engine "bff/internal/persons-engine"
	route_engine "bff/internal/route-engine"
)

// RouteAnchorEngine is the public facade for the route-relative anchor scheduler.
type RouteAnchorEngine struct {
	sched *RouteScheduler
}

// NewRouteAnchorEngine constructs a RouteAnchorEngine. Call Start after construction.
func NewRouteAnchorEngine(
	hazardScript []HazardAnchor,
	personScript []PersonAnchor,
	ctx context.Context,
	zoneRepo conditions_engine.ZoneRepository,
	cEngine *conditions_engine.ConditionsEngine,
	hub *conditions_engine.Hub,
	arRepo active_route_engine.ActiveRouteRepository,
	personRepo persons_engine.PersonRepository,
	demoStore demo_engine.DemoSessionStore,
) *RouteAnchorEngine {
	sched := newRouteScheduler(hazardScript, personScript, ctx, zoneRepo, cEngine, hub, arRepo, personRepo, demoStore)
	return &RouteAnchorEngine{sched: sched}
}

// Start begins the background TTL cleaner. Must be called once after construction.
func (e *RouteAnchorEngine) Start(ctx context.Context) {
	e.sched.startTTLCleaner(ctx)
}

// OnHeartbeat is the demo-engine HeartbeatHookFn target. Runs in a new goroutine
// (caller must not block on it). Checks anchors that cross the new progress value.
func (e *RouteAnchorEngine) OnHeartbeat(sessionID string, progress float64) {
	e.sched.OnHeartbeat(sessionID, progress)
}

// OnRouteDeleted is the active-route-engine RouteDeletedFn target.
// Cleans up spawned hazards and persons for the deleted session.
func (e *RouteAnchorEngine) OnRouteDeleted(ctx context.Context, sessionID string) {
	e.sched.OnRouteDeleted(sessionID)
}

// ClearSession explicitly tears down a session's route-anchor state — spawned
// zones, spawned persons, and scheduler indices (sessions + triggered maps).
// Idempotent: unknown sessionID is a no-op. Intended for frontend "end demo"
// signals where waiting for the 60s heartbeat TTL would leave stale zones on
// the map and stale persons in /api/vulnerable-persons responses.
func (e *RouteAnchorEngine) ClearSession(ctx context.Context, sessionID string) {
	e.sched.OnRouteDeleted(sessionID)
}

// ClearSessionHTTP handles POST /api/demo/route-anchor/clear. Reads the
// volunteer session from the X-Volunteer-Session header, returns 400 if
// missing, otherwise delegates to ClearSession and returns 204.
func (e *RouteAnchorEngine) ClearSessionHTTP(w http.ResponseWriter, r *http.Request) {
	session := r.Header.Get("X-Volunteer-Session")
	if session == "" {
		http.Error(w, "X-Volunteer-Session header is required", http.StatusBadRequest)
		return
	}
	e.ClearSession(r.Context(), session)
	w.WriteHeader(http.StatusNoContent)
}

// originAnchorSpawnRequest is the JSON body for POST /api/demo/origin-anchor/spawn.
type originAnchorSpawnRequest struct {
	Origin struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	} `json:"origin"`
	// AutoExpand seeds the session's runtime gate. Optional; defaults false
	// so expand-behavior templates stay frozen at baseline until the user
	// toggles Auto-expand on (which fires /api/demo/origin-anchor/auto-expand).
	AutoExpand bool `json:"auto_expand,omitempty"`
}

// originAnchorAutoExpandRequest is the body for POST /api/demo/origin-anchor/auto-expand.
type originAnchorAutoExpandRequest struct {
	Enabled bool `json:"enabled"`
}

// SpawnOriginAnchorsHTTP handles POST /api/demo/origin-anchor/spawn.
// Spawns the four production templates around the supplied origin for this
// session. Idempotent: prior origin-anchor zones for the session are cleared
// before re-spawning.
func (e *RouteAnchorEngine) SpawnOriginAnchorsHTTP(w http.ResponseWriter, r *http.Request) {
	session := r.Header.Get("X-Volunteer-Session")
	if session == "" {
		http.Error(w, "X-Volunteer-Session header is required", http.StatusBadRequest)
		return
	}
	var req originAnchorSpawnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	// Coarse coord validation — guard against obvious garbage.
	if req.Origin.Lat < -90 || req.Origin.Lat > 90 || req.Origin.Lng < -180 || req.Origin.Lng > 180 {
		http.Error(w, "origin lat/lng out of range", http.StatusBadRequest)
		return
	}
	e.sched.SpawnOriginAnchors(r.Context(), session, LatLng{Lat: req.Origin.Lat, Lng: req.Origin.Lng}, req.AutoExpand)
	w.WriteHeader(http.StatusNoContent)
}

// SetOriginAnchorAutoExpandHTTP handles POST /api/demo/origin-anchor/auto-expand.
// Flips the runtime gate read by runOriginExpansion each tick — pauses or
// resumes expansion for origin-anchor zones without re-spawning (which would
// reset polygon size and produce a visual jump).
func (e *RouteAnchorEngine) SetOriginAnchorAutoExpandHTTP(w http.ResponseWriter, r *http.Request) {
	session := r.Header.Get("X-Volunteer-Session")
	if session == "" {
		http.Error(w, "X-Volunteer-Session header is required", http.StatusBadRequest)
		return
	}
	var req originAnchorAutoExpandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	e.sched.SetOriginAnchorAutoExpand(session, req.Enabled)
	w.WriteHeader(http.StatusNoContent)
}

// ClearOriginAnchorsHTTP handles POST /api/demo/origin-anchor/clear.
// Removes only the session's origin-anchor zones (route-anchor / person
// state is untouched). Idempotent.
func (e *RouteAnchorEngine) ClearOriginAnchorsHTTP(w http.ResponseWriter, r *http.Request) {
	session := r.Header.Get("X-Volunteer-Session")
	if session == "" {
		http.Error(w, "X-Volunteer-Session header is required", http.StatusBadRequest)
		return
	}
	e.sched.ClearOriginAnchors(r.Context(), session)
	w.WriteHeader(http.StatusNoContent)
}

// SetRouteRecomputeHook registers fn invoked by the β expansion goroutine
// when a growing zone first intersects an active route — re-uses D5 in the
// active-route-engine without duplicating its logic here. Wired in main.go
// to arEngine.OnHazardActivated.
func (e *RouteAnchorEngine) SetRouteRecomputeHook(fn conditions_engine.HazardActivatedFn) {
	e.sched.routeRecomputeHook = fn
}

// OnRerouteAccepted is the active-route-engine RerouteAcceptedFn target.
// Replaces the cached polyline for the session and resets script walk indices
// so future anchors fire against the new geometry. GeoJSON [lng,lat] ordering
// is converted to internal LatLng on the way in.
func (e *RouteAnchorEngine) OnRerouteAccepted(ctx context.Context, sessionID string, geom route_engine.GeoJSONGeometry) {
	polyline := make([]LatLng, 0, len(geom.Coordinates))
	for _, c := range geom.Coordinates {
		if len(c) < 2 {
			continue
		}
		polyline = append(polyline, LatLng{Lat: c[1], Lng: c[0]})
	}
	e.sched.OnRerouteAccepted(ctx, sessionID, polyline)
}

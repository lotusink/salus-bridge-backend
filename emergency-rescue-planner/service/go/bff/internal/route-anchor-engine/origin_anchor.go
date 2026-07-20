package route_anchor_engine

import (
	"context"
	"fmt"
	"math"
	"time"

	conditions_engine "bff/internal/conditions-engine"
)

// =============================================================================
// Origin-Anchor mode
// =============================================================================
//
// When Hazard Simulation is ON but the user hasn't yet committed to a route,
// hazards spawn around the user's current origin instead of on a route
// polyline. Four templates cover the design matrix: {cover origin / off
// origin} × {static / expanding}. The expansion goroutine here is simpler
// than runExpansion — no D5 intersect check (no route to reroute), just
// grow polygon and broadcast hazard_updated each step.
//
// When the user enters route mode (active route registered), the frontend
// invokes ClearOriginAnchors and the route-anchor scheduler takes over.

// OriginAnchorTemplate is a Hazard descriptor positioned relative to the
// user's origin (rather than a route polyline). Each template's centroid is
// offset by (CentroidNorthM, CentroidEastM) metres from origin; polygon
// vertices are then placed by PolygonOffsets relative to that centroid.
type OriginAnchorTemplate struct {
	ID                    string
	CentroidNorthM        float64 // metres north (+) / south (-) of origin
	CentroidEastM         float64 // metres east (+) / west (-) of origin
	PolygonOffsets        []OffsetM
	Level                 conditions_engine.ZoneLevel
	Label                 string
	Behavior              string  // "" / "static" — no expansion; "expand" — runOriginExpansion
	ExpansionTargetFactor float64 // only honoured when Behavior == "expand"
	ExpansionStepMs       int     // only honoured when Behavior == "expand"
}

// DefaultOriginAnchorTemplates is the production set used when Hazard Sim is
// ON and the user has no active route yet. Covers all four
// {cover/off}×{static/expand} combinations.
var DefaultOriginAnchorTemplates = []OriginAnchorTemplate{
	{
		ID:             "cover-static",
		CentroidNorthM: 0, CentroidEastM: 0, // covers origin
		Level:    conditions_engine.ZoneLevelHigh,
		Label:    "Active Hazard — You Are Inside",
		Behavior: "static",
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 180}, {NorthM: 127, EastM: 127},
			{NorthM: 180, EastM: 0}, {NorthM: 127, EastM: -127},
			{NorthM: 0, EastM: -180}, {NorthM: -127, EastM: -127},
			{NorthM: -180, EastM: 0}, {NorthM: -127, EastM: 127},
		},
	},
	{
		// Offset SW so this template isn't visually concentric with cover-
		// static. Centroid distance from origin = √(180² + 180²) ≈ 254 m.
		// At peak factor 2.8, base 100 m → 280 m radius, which just barely
		// engulfs origin — preserves the "hazard intensifies and reaches
		// you" demo arc while keeping the polygon visually distinct from
		// cover-static throughout the expansion.
		ID:             "cover-expand",
		CentroidNorthM: -180, CentroidEastM: -180,
		Level:                 conditions_engine.ZoneLevelHigh,
		Label:                 "Hazard Intensifying — Closing In",
		Behavior:              "expand",
		ExpansionTargetFactor: 2.8,
		ExpansionStepMs:       2000,
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 100}, {NorthM: 70, EastM: 70},
			{NorthM: 100, EastM: 0}, {NorthM: 70, EastM: -70},
			{NorthM: 0, EastM: -100}, {NorthM: -70, EastM: -70},
			{NorthM: -100, EastM: 0}, {NorthM: -70, EastM: 70},
		},
	},
	{
		ID:             "off-static",
		CentroidNorthM: 350, CentroidEastM: 350, // NE of origin, doesn't cover
		Level:    conditions_engine.ZoneLevelHigh,
		Label:    "Nearby Hazard — Stable",
		Behavior: "static",
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 100}, {NorthM: 70, EastM: 70},
			{NorthM: 100, EastM: 0}, {NorthM: 70, EastM: -70},
			{NorthM: 0, EastM: -100}, {NorthM: -70, EastM: -70},
			{NorthM: -100, EastM: 0}, {NorthM: -70, EastM: 70},
		},
	},
	{
		ID:             "off-expand",
		CentroidNorthM: 350, CentroidEastM: -350, // NW of origin (N 350, W 350)
		Level:                 conditions_engine.ZoneLevelHigh,
		Label:                 "Distant Risk — Escalating",
		Behavior:              "expand",
		ExpansionTargetFactor: 2.5,
		ExpansionStepMs:       2000,
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 80}, {NorthM: 56, EastM: 56},
			{NorthM: 80, EastM: 0}, {NorthM: 56, EastM: -56},
			{NorthM: 0, EastM: -80}, {NorthM: -56, EastM: -56},
			{NorthM: -80, EastM: 0}, {NorthM: -56, EastM: 56},
		},
	},
}

// applyMetricOffset displaces an origin (lat, lng) by (northM, eastM) metres,
// using a flat-earth approximation valid at the 100-1000m scale this mode
// operates at. Mirrors the math route-anchor-engine uses for perpendicular
// offsets.
func applyMetricOffset(origin LatLng, northM, eastM float64) LatLng {
	const metresPerDegLat = 111_320.0
	metresPerDegLng := metresPerDegLat * math.Cos(origin.Lat*math.Pi/180)
	if metresPerDegLng == 0 {
		metresPerDegLng = metresPerDegLat // pathological pole case
	}
	return LatLng{
		Lat: origin.Lat + northM/metresPerDegLat,
		Lng: origin.Lng + eastM/metresPerDegLng,
	}
}

// SpawnOriginAnchors is the public entry: spawns the production
// DefaultOriginAnchorTemplates around the given origin for this session.
// autoExpand seeds the session's runtime gate — set false to spawn expand-
// behavior templates frozen at baseline; flip via SetOriginAnchorAutoExpand.
// Idempotent — calling again on the same session replaces the prior set.
func (s *RouteScheduler) SpawnOriginAnchors(ctx context.Context, sessionID string, origin LatLng, autoExpand bool) {
	s.spawnOriginAnchorsImpl(ctx, sessionID, origin, autoExpand, DefaultOriginAnchorTemplates)
}

// SetOriginAnchorAutoExpand flips the runtime gate read by
// runOriginExpansion each tick. true → expand-behavior templates resume
// growing; false → freeze at current factor. No-op for unknown sessions.
func (s *RouteScheduler) SetOriginAnchorAutoExpand(sessionID string, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[sessionID]; ok {
		sess.OriginAnchorAutoExpand = enabled
	}
}

// spawnOriginAnchorsImpl is the testable inner entry — allows injecting a
// custom template list with millisecond-scale expansion for tests.
func (s *RouteScheduler) spawnOriginAnchorsImpl(ctx context.Context, sessionID string, origin LatLng, autoExpand bool, templates []OriginAnchorTemplate) {
	// Clear any prior origin-anchor zones for this session (idempotency).
	s.ClearOriginAnchors(ctx, sessionID)

	// Ensure session exists in the scheduler map; seed the runtime gate.
	s.mu.Lock()
	if _, exists := s.sessions[sessionID]; !exists {
		s.sessions[sessionID] = &RouteSchedulerSession{SessionID: sessionID}
	}
	s.sessions[sessionID].OriginAnchorAutoExpand = autoExpand
	s.mu.Unlock()

	for _, tmpl := range templates {
		centroid := applyMetricOffset(origin, tmpl.CentroidNorthM, tmpl.CentroidEastM)
		poly := ApplyPolygonOffsets(centroid, tmpl.PolygonOffsets)
		zoneID := fmt.Sprintf("oa-%s-%s", sessionID, tmpl.ID)
		zone := conditions_engine.Zone{
			ID:     zoneID,
			Level:  tmpl.Level,
			Label:  tmpl.Label,
			Source: "origin-anchor",
			ActiveAlert: tmpl.Level == conditions_engine.ZoneLevelHigh ||
				tmpl.Level == conditions_engine.ZoneLevelActive,
			Polygon:   poly,
			UpdatedAt: time.Now(),
		}
		if err := s.cEngine.ActivateZone(s.ctx, zone); err != nil {
			continue
		}
		s.mu.Lock()
		s.sessions[sessionID].SpawnedOriginAnchorIDs = append(
			s.sessions[sessionID].SpawnedOriginAnchorIDs, zoneID,
		)
		s.mu.Unlock()

		if tmpl.Behavior == "expand" {
			go s.runOriginExpansion(sessionID, zoneID, tmpl, centroid)
		}
	}
}

// ClearOriginAnchors removes only origin-anchor zones for the session. Does
// NOT touch SpawnedZoneIDs (route-anchor) or SpawnedPersonIDs. Idempotent.
func (s *RouteScheduler) ClearOriginAnchors(ctx context.Context, sessionID string) {
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if !ok || len(sess.SpawnedOriginAnchorIDs) == 0 {
		s.mu.Unlock()
		return
	}
	ids := append([]string(nil), sess.SpawnedOriginAnchorIDs...)
	sess.SpawnedOriginAnchorIDs = nil
	s.mu.Unlock()

	for _, zid := range ids {
		_ = s.zoneRepo.DeleteZone(s.ctx, zid)
		s.hub.Broadcast(makeLocalClearedEnvelope(zid))
	}
}

// runOriginExpansion grows an origin-anchor zone's polygon over time.
// Simpler than route-anchor-engine.runExpansion — no D5 intersect, no
// reroute hook. Exits on: target reached, ctx cancel, session deleted, or
// zone removed from SpawnedOriginAnchorIDs (e.g. ClearOriginAnchors).
func (s *RouteScheduler) runOriginExpansion(sessionID, zoneID string, tmpl OriginAnchorTemplate, centroid LatLng) {
	target := tmpl.ExpansionTargetFactor
	if target <= 1 {
		return
	}
	stepMs := tmpl.ExpansionStepMs
	if stepMs <= 0 {
		stepMs = 2000
	}

	const steps = 10
	factorStep := (target - 1.0) / steps
	factor := 1.0

	ticker := time.NewTicker(time.Duration(stepMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-s.ctx.Done():
			return
		}

		// Resurrection guard + auto-expand gate read.
		s.mu.Lock()
		sess, sessAlive := s.sessions[sessionID]
		stillSpawned := false
		autoExpand := false
		if sessAlive {
			autoExpand = sess.OriginAnchorAutoExpand
			for _, z := range sess.SpawnedOriginAnchorIDs {
				if z == zoneID {
					stillSpawned = true
					break
				}
			}
		}
		s.mu.Unlock()
		if !sessAlive || !stillSpawned {
			return
		}
		// Gate: if user has Auto-expand toggle off, freeze polygon at current
		// factor — keep the goroutine alive so a later toggle ON resumes
		// growth without a baseline reset (which would look like a jump).
		if !autoExpand {
			continue
		}

		factor += factorStep
		if factor > target {
			factor = target
		}

		scaled := make([]OffsetM, len(tmpl.PolygonOffsets))
		for i, off := range tmpl.PolygonOffsets {
			scaled[i] = OffsetM{NorthM: off.NorthM * factor, EastM: off.EastM * factor}
		}
		newPoly := ApplyPolygonOffsets(centroid, scaled)

		updatedZone := conditions_engine.Zone{
			ID:     zoneID,
			Level:  tmpl.Level,
			Label:  tmpl.Label,
			Source: "origin-anchor",
			ActiveAlert: tmpl.Level == conditions_engine.ZoneLevelHigh ||
				tmpl.Level == conditions_engine.ZoneLevelActive,
			Polygon:   newPoly,
			UpdatedAt: time.Now(),
		}
		if err := s.cEngine.UpdateZone(s.ctx, updatedZone); err != nil {
			return
		}

		if factor >= target {
			return
		}
	}
}

package route_anchor_engine

import (
	conditions_engine "bff/internal/conditions-engine"
	persons_engine "bff/internal/persons-engine"
)

// OffsetM is a relative offset from a centroid point, in metres.
type OffsetM struct {
	NorthM float64 // metres north (+) or south (-)
	EastM  float64 // metres east (+) or west (-)
}

// Direction is a bearing in degrees, 0 = true north, increasing clockwise.
type Direction float64

// LatLng is a geographic point; always stored as (lat, lng), NOT GeoJSON order.
type LatLng struct {
	Lat float64
	Lng float64
}

// HazardAnchor is a route-relative hazard template.
// TriggerAtProgress and CentroidAtProgress are INDEPENDENT fields:
//
//	trigger decides WHEN the anchor fires (heartbeat crosses it);
//	centroid decides WHERE the hazard centroid sits on the polyline.
//
// Example: trigger=0.30, centroid=0.50 → user reaches 30%, hazard appears
// at 50% (ahead) → D5 finds user outside polygon → reroute_proposal fires.
type HazardAnchor struct {
	ID                   string
	TriggerAtProgress    float64   // 0.0–1.0
	CentroidAtProgress   float64   // 0.0–1.0
	PerpendicularOffsetM float64   // metres right of travel; negative = left
	PolygonOffsets       []OffsetM // vertices relative to centroid
	Level                conditions_engine.ZoneLevel
	Label                string
	Source               string

	// LifecycleMs > 0 turns the anchor into a δ-type event: after the zone is
	// spawned, a goroutine waits LifecycleMs milliseconds then calls
	// DeleteZone + broadcasts hazard_cleared. Zero (default) means the zone
	// persists until route deletion.
	LifecycleMs int

	// β expansion: when Behavior == "expand" AND session.AutoExpandEnabled is
	// true at anchor fire time, a goroutine grows the spawned polygon by
	// scaling its offsets linearly from 1× to ExpansionTargetFactor over a
	// sequence of ExpansionStepMs ticks. On each tick it broadcasts
	// hazard_updated and checks whether the new polygon intersects the
	// session's active route — on intersect, it fires the routeRecomputeHook
	// exactly once (D5 reroute) and exits.
	Behavior              string  // "" (default = static) | "expand"
	ExpansionTargetFactor float64 // e.g. 2.5; ignored unless Behavior == "expand"
	ExpansionStepMs       int     // e.g. 2000; defaults to 2000 if ≤ 0
}

// PersonTemplate holds the static person fields copied into a spawned Person.
// SupportGuideID follows persons_engine.Person convention: nil means no guide.
type PersonTemplate struct {
	Label            string
	Needs            []string
	NeedsSummary     string
	CtaLabel         string
	SupportGuideID   *string
	DestinationLabel string
}

// PersonAnchor is a route-relative person template.
// Same trigger/centroid independence as HazardAnchor.
type PersonAnchor struct {
	ID                   string
	TriggerAtProgress    float64
	CentroidAtProgress   float64
	PerpendicularOffsetM float64
	PersonTemplate       PersonTemplate
}

// PersonAddedPayloadWire is the WS payload for person_added events.
type PersonAddedPayloadWire struct {
	Person        persons_engine.PersonWire `json:"person"`
	ActiveRouteID string                    `json:"active_route_id"`
}

// PersonRemovedPayloadWire is the WS payload for person_removed events.
type PersonRemovedPayloadWire struct {
	PersonID string `json:"person_id"`
}

// RouteSchedulerSession holds per-client scheduler state.
// Fields are accessed only while RouteScheduler.mu is held.
type RouteSchedulerSession struct {
	SessionID        string
	ActiveRouteID    string
	Polyline         []LatLng // converted from GeoJSON at session init
	NextHazardIdx    int
	NextPersonIdx    int
	SpawnedZoneIDs   []string
	SpawnedPersonIDs []string
	// SpawnedOriginAnchorIDs tracks zones spawned by the "no-route" Origin
	// Anchor mode (Source="origin-anchor"). Kept separate from
	// SpawnedZoneIDs so we can clear just the origin-anchor subset when
	// the user transitions into route mode without touching route-anchor.
	SpawnedOriginAnchorIDs []string

	// OriginAnchorAutoExpand controls whether origin-anchor expand-behavior
	// templates actually grow each tick. Read by runOriginExpansion on every
	// tick — flipping it via SetOriginAnchorAutoExpand pauses or resumes
	// expansion without re-spawning (which would reset polygon size and
	// produce a visual "jump"). Kept separate from
	// demo_engine.DemoSessionStore.AutoExpandEnabled because the latter has
	// a 60s heartbeat TTL that would evict origin-anchor sessions (which
	// don't emit heartbeats — Movement Sim is off in origin-anchor mode).
	OriginAnchorAutoExpand bool
}

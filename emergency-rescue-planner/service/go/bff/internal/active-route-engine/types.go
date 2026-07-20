package active_route_engine

import (
	"context"
	"errors"
	"time"

	route_engine "bff/internal/route-engine"
)

var (
	ErrActiveRouteNotFound = errors.New("active route not found")
	ErrProposalNotFound    = errors.New("reroute proposal not found")
	ErrProposalExpired     = errors.New("reroute proposal expired")
)

const ProposalTTL = 5 * time.Minute

// --- Domain types (no JSON tags) ---

type LatLng struct {
	Lat float64
	Lng float64
}

type ActiveRoute struct {
	ID               string
	VolunteerSession string
	Origin           LatLng
	Destination      LatLng
	Waypoints        []LatLng
	Geometry         route_engine.GeoJSONGeometry
	DurationSeconds  float64
	DistanceMetres   float64
	BBox             []float64
	AvoidZoneIDs     []string
	Transport        string
	RegisteredAt     time.Time
}

type RerouteProposal struct {
	ID               string
	ActiveRouteID    string
	TriggeringZoneID string
	NewRoute         route_engine.RouteData
	EtaDeltaSeconds  float64
	CreatedAt        time.Time
}

// --- Wire types (JSON tags) ---

type LatLngWire struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type ActiveRouteWire struct {
	ID              string                       `json:"id"`
	Origin          LatLngWire                   `json:"origin"`
	Destination     LatLngWire                   `json:"destination"`
	Geometry        route_engine.GeoJSONGeometry `json:"geometry"`
	DurationSeconds float64                      `json:"duration_seconds"`
	DistanceMetres  float64                      `json:"distance_metres"`
	BBox            []float64                    `json:"bbox,omitempty"`
	RegisteredAt    string                       `json:"registered_at"`
}

type RegisterRouteRequest struct {
	Origin          LatLngWire                   `json:"origin"`
	Destination     LatLngWire                   `json:"destination"`
	Waypoints       []LatLngWire                 `json:"waypoints"`
	Geometry        route_engine.GeoJSONGeometry `json:"geometry"`
	DurationSeconds float64                      `json:"duration_seconds"`
	DistanceMetres  float64                      `json:"distance_metres"`
	BBox            []float64                    `json:"bbox"`
	AvoidZoneIDs    []string                     `json:"avoid_zone_ids"`
	Transport       string                       `json:"transport"`
}

type RegisterRouteResponseData struct {
	ActiveRoute ActiveRouteWire `json:"active_route"`
}
type RegisterRouteResponse struct {
	Data RegisterRouteResponseData `json:"data"`
}

type AcceptRerouteRequest struct {
	ProposalID string `json:"proposal_id"`
}

type AcceptRerouteResponseData struct {
	ActiveRoute ActiveRouteWire `json:"active_route"`
}
type AcceptRerouteResponse struct {
	Data AcceptRerouteResponseData `json:"data"`
}

// RerouteProposalPayloadWire is the WS payload for reroute_proposal events.
type RerouteProposalPayloadWire struct {
	ProposalID       string                 `json:"proposal_id"`
	ActiveRouteID    string                 `json:"active_route_id"`
	TriggeringZoneID string                 `json:"triggering_zone_id"`
	EtaDeltaSeconds  float64                `json:"eta_delta_seconds"`
	Route            route_engine.RouteData `json:"route"`
}

// RouteWarningPayloadWire is the WS payload for route_warning events.
type RouteWarningPayloadWire struct {
	ActiveRouteID    string `json:"active_route_id"`
	TriggeringZoneID string `json:"triggering_zone_id"`
	Message          string `json:"message"`
}

// RouteDeletedFn is called after an active route is successfully deleted.
// sessionID is the X-Volunteer-Session value of the deleted route owner.
// Used by route-anchor-engine to clean up route-relative spawned hazards and persons.
type RouteDeletedFn func(ctx context.Context, sessionID string)

// RerouteAcceptedFn is called after an active route is successfully replaced
// by AcceptReroute. sessionID owns the route; newGeometry is the fresh
// polyline (GeoJSON [lng,lat] order) ORS returned at accept time. Used by
// route-anchor-engine to refresh its cached polyline so subsequent anchor
// projections use the new geometry; already-fired anchors stay fired.
type RerouteAcceptedFn func(ctx context.Context, sessionID string, newGeometry route_engine.GeoJSONGeometry)

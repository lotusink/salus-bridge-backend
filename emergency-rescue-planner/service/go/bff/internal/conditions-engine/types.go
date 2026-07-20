package conditions_engine

import (
	"context"
	"time"
)

// ZoneLevel represents the risk severity of a risk zone.
type ZoneLevel string

const (
	ZoneLevelUnknown ZoneLevel = "unknown"
	ZoneLevelLow     ZoneLevel = "low"
	ZoneLevelMedium  ZoneLevel = "medium"
	ZoneLevelHigh    ZoneLevel = "high"
	ZoneLevelActive  ZoneLevel = "active"
)

// Zone is the domain representation of a risk zone.
// No JSON or DB tags — wire conversion is done in engine.go.
type Zone struct {
	ID          string
	Level       ZoneLevel
	Label       string
	Source      string
	UpdatedAt   time.Time
	ActiveAlert bool
	Polygon     [][2]float64 // [lat, lng] Leaflet convention; NOT GeoJSON [lng, lat]
}

// --- Wire types (JSON tags; used only in HTTP/WS payloads) ---

// ZoneWire is the JSON representation of Zone for HTTP responses and WS payloads.
type ZoneWire struct {
	ID          string       `json:"id"`
	Level       string       `json:"level"`
	Label       string       `json:"label"`
	Source      string       `json:"source"`
	UpdatedAt   string       `json:"updated_at"`
	ActiveAlert bool         `json:"active_alert"`
	Polygon     [][2]float64 `json:"polygon"`
}

// GetRiskZonesData is the data envelope for GET /api/conditions/risk-zones.
type GetRiskZonesData struct {
	Zones []ZoneWire `json:"zones"`
}

// GetRiskZonesResponse is the full response for GET /api/conditions/risk-zones.
type GetRiskZonesResponse struct {
	Data GetRiskZonesData `json:"data"`
}

// WsEnvelopeWire is the JSON envelope for all WS server→client events.
type WsEnvelopeWire struct {
	Type    string      `json:"type"`
	Ts      string      `json:"ts"`
	Payload interface{} `json:"payload"`
}

// HazardActivatedPayloadWire is the payload for hazard_activated WS events.
type HazardActivatedPayloadWire struct {
	Zone ZoneWire `json:"zone"`
}

// HazardUpdatedPayloadWire is the payload for hazard_updated WS events.
type HazardUpdatedPayloadWire struct {
	Zone ZoneWire `json:"zone"`
}

// HazardClearedPayloadWire is the payload for hazard_cleared WS events.
type HazardClearedPayloadWire struct {
	ZoneID string `json:"zone_id"`
}

// HazardActivatedFn is the callback type invoked after a zone is activated.
// Used by active-route-engine to trigger D5 reroute decisions.
type HazardActivatedFn func(ctx context.Context, zone Zone)

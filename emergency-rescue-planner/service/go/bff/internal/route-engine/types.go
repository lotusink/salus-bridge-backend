package route_engine

// LatLng is a geographic coordinate pair.
type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// RouteRequest is the JSON body for POST /api/route/calculate.
type RouteRequest struct {
	Origin       LatLng   `json:"origin"`
	Destination  LatLng   `json:"destination"`
	Transport    string   `json:"transport"`
	AvoidZoneIDs []string `json:"avoid_zone_ids"`
	Waypoints    []LatLng `json:"waypoints"`
}

// RouteStep is a single turn-by-turn instruction step.
type RouteStep struct {
	Instruction   string  `json:"instruction"`
	DistanceM     float64 `json:"distance_m"`
	DurationSec   float64 `json:"duration_sec"`
	ManeuverType  string  `json:"maneuver_type"`
	WayPointIndex int     `json:"way_point_index"`
}

// GeoJSONGeometry represents a GeoJSON geometry object.
type GeoJSONGeometry struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

// RouteData is the route object nested inside the response data envelope.
type RouteData struct {
	Geometry        GeoJSONGeometry `json:"geometry"`
	DurationSeconds float64         `json:"duration_seconds"`
	DistanceMetres  float64         `json:"distance_metres"`
	BBox            []float64       `json:"bbox,omitempty"`
	Steps           []RouteStep     `json:"steps"`
}

// RouteResponseData is the value of the `data` field.
type RouteResponseData struct {
	Route           RouteData `json:"route"`
	AffectedZoneIDs []string  `json:"affected_zone_ids"`
}

// RouteResponse is the full response envelope.
type RouteResponse struct {
	Data RouteResponseData `json:"data"`
}

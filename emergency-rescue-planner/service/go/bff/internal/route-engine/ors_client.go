package route_engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// orsManeuverType maps ORS integer step type codes to contract maneuver_type strings.
var orsManeuverType = map[int]string{
	0:  "depart",
	1:  "left",
	2:  "right",
	3:  "sharp_left",
	4:  "sharp_right",
	5:  "slight_left",
	6:  "slight_right",
	7:  "straight",
	8:  "enter_roundabout",
	9:  "exit_roundabout",
	10: "u_turn",
	11: "arrive",
	12: "depart",
	13: "straight",
}

// orsTransportProfile maps contract transport values to ORS profile strings.
var orsTransportProfile = map[string]string{
	"drive":   "driving-car",
	"walk":    "foot-walking",
	"bicycle": "cycling-regular",
}

// ORSAvoidPolygons is a GeoJSON MultiPolygon used in the ORS avoid_polygons field.
type ORSAvoidPolygons struct {
	Type        string           `json:"type"`        // "MultiPolygon"
	Coordinates [][][][2]float64 `json:"coordinates"` // [polygon][ring][point][lng,lat]
}

// ORSOptions wraps optional routing parameters for ORS v2.
// ORS v2 POST /v2/directions/{profile}/geojson requires avoid_polygons nested
// under "options"; placing it at the request root causes HTTP 502.
type ORSOptions struct {
	AvoidPolygons *ORSAvoidPolygons `json:"avoid_polygons,omitempty"`
}

// ORSDirectionsRequest is the body sent to the ORS directions endpoint.
type ORSDirectionsRequest struct {
	Coordinates [][2]float64 `json:"coordinates"`
	Options     *ORSOptions  `json:"options,omitempty"`
}

// ORSStep is a single step inside an ORS route segment.
type ORSStep struct {
	Instruction string  `json:"instruction"`
	Distance    float64 `json:"distance"`
	Duration    float64 `json:"duration"`
	Type        int     `json:"type"`
	WayPoints   []int   `json:"way_points"`
}

// ORSSegment is a route segment containing steps.
type ORSSegment struct {
	Steps []ORSStep `json:"steps"`
}

// ORSRouteSummary holds distance and duration for the whole route.
type ORSRouteSummary struct {
	Distance float64 `json:"distance"`
	Duration float64 `json:"duration"`
}

// ORSRouteProperties contains ORS feature properties.
type ORSRouteProperties struct {
	Summary  ORSRouteSummary `json:"summary"`
	Segments []ORSSegment    `json:"segments"`
	// BBox is always empty in ORS responses — bbox is at FeatureCollection level, not feature level.
	BBox []float64 `json:"bbox"`
}

// ORSFeature is a single GeoJSON Feature in the ORS response.
type ORSFeature struct {
	Geometry   GeoJSONGeometry    `json:"geometry"`
	Properties ORSRouteProperties `json:"properties"`
}

// ORSDirectionsResponse is the top-level ORS GeoJSON FeatureCollection.
type ORSDirectionsResponse struct {
	Features []ORSFeature `json:"features"`
	BBox     []float64    `json:"bbox"`
}

type ORSClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewORSClient(apiKey string) *ORSClient {
	return &ORSClient{
		apiKey:     apiKey,
		baseURL:    "https://api.openrouteservice.org",
		httpClient: &http.Client{},
	}
}

// Directions calls the ORS directions/{profile}/geojson endpoint.
// Returns the full ORSDirectionsResponse on success. Returns error on non-200 or parse failure.
func (c *ORSClient) Directions(ctx context.Context, profile string, req ORSDirectionsRequest) (*ORSDirectionsResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal ORS request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v2/directions/"+profile+"/geojson", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build ORS request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ORS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ORS %d: %s", resp.StatusCode, string(b))
	}

	var orsResp ORSDirectionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&orsResp); err != nil {
		return nil, fmt.Errorf("decode ORS response: %w", err)
	}

	if len(orsResp.Features) == 0 {
		return nil, fmt.Errorf("ORS returned empty feature collection")
	}

	return &orsResp, nil
}

// BuildRouteData constructs a RouteData from an ORS directions response.
// bbox is read from the FeatureCollection level (orsResp.BBox), not from feature
// properties — ORS does not populate the per-feature bbox field.
func BuildRouteData(orsResp *ORSDirectionsResponse) RouteData {
	feature := orsResp.Features[0]
	return RouteData{
		Geometry:        feature.Geometry,
		DurationSeconds: feature.Properties.Summary.Duration,
		DistanceMetres:  feature.Properties.Summary.Distance,
		BBox:            orsResp.BBox,
		Steps:           flattenSteps(feature.Properties.Segments),
	}
}

// ProfileFromTransport maps a contract transport string to an ORS profile.
// Returns "" for unknown values.
func ProfileFromTransport(transport string) string {
	return orsTransportProfile[transport]
}

func orsStepManeuver(t int) string {
	if s, ok := orsManeuverType[t]; ok {
		return s
	}
	return "unknown"
}

func flattenSteps(segments []ORSSegment) []RouteStep {
	var steps []RouteStep
	for _, seg := range segments {
		for _, s := range seg.Steps {
			wpIdx := 0
			if len(s.WayPoints) > 0 {
				wpIdx = s.WayPoints[0]
			}
			steps = append(steps, RouteStep{
				Instruction:   s.Instruction,
				DistanceM:     s.Distance,
				DurationSec:   s.Duration,
				ManeuverType:  orsStepManeuver(s.Type),
				WayPointIndex: wpIdx,
			})
		}
	}
	return steps
}

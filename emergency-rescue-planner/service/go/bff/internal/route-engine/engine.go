package route_engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// RouteEngine handles POST /api/route/calculate.
type RouteEngine struct {
	apiKey string
	ors    *ORSClient
	zones  ZoneStore
}

// NewRouteEngine constructs a RouteEngine. apiKey may be empty; the handler
// returns 503 when empty so the frontend fallback triggers cleanly.
func NewRouteEngine(apiKey string, ors *ORSClient, zones ZoneStore) *RouteEngine {
	return &RouteEngine{apiKey: apiKey, ors: ors, zones: zones}
}

// CalculateRoute handles POST /api/route/calculate.
//
// @Summary      Calculate a road-network route
// @Description  Returns a road-network route with turn-by-turn steps. Requires ORS_API_KEY.
// @Tags         routing
// @Accept       json
// @Produce      json
// @Param        request body RouteRequest true "Route Request"
// @Success      200 {object} RouteResponse
// @Failure      400 {string} string "invalid request"
// @Failure      503 {string} string "routing service unavailable"
// @Router       /api/route/calculate [post]
func (e *RouteEngine) CalculateRoute(w http.ResponseWriter, r *http.Request) {
	var req RouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := req.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if e.apiKey == "" {
		http.Error(w, `{"error":"routing service unavailable: ORS_API_KEY not set"}`, http.StatusServiceUnavailable)
		return
	}

	// Build ORS coordinates: origin → waypoints → destination (GeoJSON [lng, lat])
	coords := make([][2]float64, 0, 2+len(req.Waypoints))
	coords = append(coords, [2]float64{req.Origin.Lng, req.Origin.Lat})
	for _, wp := range req.Waypoints {
		coords = append(coords, [2]float64{wp.Lng, wp.Lat})
	}
	coords = append(coords, [2]float64{req.Destination.Lng, req.Destination.Lat})

	// Build avoid_polygons under options (not at request root — ORS v2 requirement)
	orsReq := ORSDirectionsRequest{Coordinates: coords}
	if len(req.AvoidZoneIDs) > 0 {
		var multiPoly [][][][2]float64
		for _, id := range req.AvoidZoneIDs {
			poly, ok := e.zones.GetPolygon(id)
			if !ok {
				continue
			}
			ring := make([][2]float64, len(poly)+1)
			for i, ll := range poly {
				ring[i] = [2]float64{ll.Lng, ll.Lat}
			}
			ring[len(poly)] = ring[0] // close ring
			multiPoly = append(multiPoly, [][][2]float64{ring})
		}
		if len(multiPoly) > 0 {
			orsReq.Options = &ORSOptions{
				AvoidPolygons: &ORSAvoidPolygons{
					Type:        "MultiPolygon",
					Coordinates: multiPoly,
				},
			}
		}
	}

	// Call ORS with 30-second timeout
	profile := orsTransportProfile[req.Transport]
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	orsResp, err := e.ors.Directions(ctx, profile, orsReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("routing failed: %v", err), http.StatusBadGateway)
		return
	}

	// Build route data (bbox from FeatureCollection level)
	routeData := BuildRouteData(orsResp)
	affected := computeAffectedZones(orsResp.Features[0].Geometry.Coordinates, req.AvoidZoneIDs, e.zones)

	resp := RouteResponse{
		Data: RouteResponseData{
			Route:           routeData,
			AffectedZoneIDs: affected,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return
	}
}

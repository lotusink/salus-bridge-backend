package active_route_engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	conditions_engine "bff/internal/conditions-engine"
	demo_engine "bff/internal/demo-engine"
	route_engine "bff/internal/route-engine"
)

// orsDirectioner is the ORS routing interface used by ActiveRouteEngine.
// *route_engine.ORSClient satisfies this interface; tests can provide a mock.
type orsDirectioner interface {
	Directions(ctx context.Context, profile string, req route_engine.ORSDirectionsRequest) (*route_engine.ORSDirectionsResponse, error)
}

// ActiveRouteEngine handles /api/routes/active endpoints.
type ActiveRouteEngine struct {
	repo            ActiveRouteRepository
	ors             orsDirectioner
	zoneRepo        conditions_engine.ZoneRepository
	hub             *conditions_engine.Hub
	routeDeleted    RouteDeletedFn
	rerouteAccepted RerouteAcceptedFn
	demoStore       demo_engine.DemoSessionStore // used to project the volunteer's current position onto the active route when computing reroutes (D5)
}

// NewActiveRouteEngine creates an ActiveRouteEngine backed by the given repository.
// Reroute dependencies (ORS client, zone repository, hub) are injected via
// SetRerouteComponents after construction.
func NewActiveRouteEngine(repo ActiveRouteRepository) *ActiveRouteEngine {
	return &ActiveRouteEngine{repo: repo}
}

// SetRouteDeletedHook registers fn to be called after every successful DeleteActiveRoute.
func (e *ActiveRouteEngine) SetRouteDeletedHook(fn RouteDeletedFn) {
	e.routeDeleted = fn
}

// SetRerouteAcceptedHook registers fn to be called after every successful
// AcceptReroute. Used by route-anchor-engine to refresh its session polyline.
func (e *ActiveRouteEngine) SetRerouteAcceptedHook(fn RerouteAcceptedFn) {
	e.rerouteAccepted = fn
}

// SetRerouteComponents wires in the reroute dependencies (ORS, zones, hub).
func (e *ActiveRouteEngine) SetRerouteComponents(
	ors orsDirectioner,
	zoneRepo conditions_engine.ZoneRepository,
	hub *conditions_engine.Hub,
) {
	e.ors = ors
	e.zoneRepo = zoneRepo
	e.hub = hub
}

// SetDemoStore wires in the demo session store used to project the volunteer's
// current position during OnHazardActivated. Optional — nil disables projection.
func (e *ActiveRouteEngine) SetDemoStore(store demo_engine.DemoSessionStore) {
	e.demoStore = store
}

func activeRouteToWire(r ActiveRoute) ActiveRouteWire {
	origin := LatLngWire{Lat: r.Origin.Lat, Lng: r.Origin.Lng}
	dest := LatLngWire{Lat: r.Destination.Lat, Lng: r.Destination.Lng}
	return ActiveRouteWire{
		ID:              r.ID,
		Origin:          origin,
		Destination:     dest,
		Geometry:        r.Geometry,
		DurationSeconds: r.DurationSeconds,
		DistanceMetres:  r.DistanceMetres,
		BBox:            r.BBox,
		RegisteredAt:    r.RegisteredAt.UTC().Format(time.RFC3339),
	}
}

// RegisterActiveRoute handles POST /api/routes/active.
func (e *ActiveRouteEngine) RegisterActiveRoute(w http.ResponseWriter, r *http.Request) {
	session := r.Header.Get("X-Volunteer-Session")
	if session == "" {
		http.Error(w, "X-Volunteer-Session header is required", http.StatusBadRequest)
		return
	}

	var req RegisterRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := req.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	waypoints := make([]LatLng, 0, len(req.Waypoints))
	for _, wp := range req.Waypoints {
		waypoints = append(waypoints, LatLng{Lat: wp.Lat, Lng: wp.Lng})
	}
	avoidIDs := req.AvoidZoneIDs
	if avoidIDs == nil {
		avoidIDs = make([]string, 0)
	}

	route := ActiveRoute{
		VolunteerSession: session,
		Origin:           LatLng{Lat: req.Origin.Lat, Lng: req.Origin.Lng},
		Destination:      LatLng{Lat: req.Destination.Lat, Lng: req.Destination.Lng},
		Waypoints:        waypoints,
		Geometry:         req.Geometry,
		DurationSeconds:  req.DurationSeconds,
		DistanceMetres:   req.DistanceMetres,
		BBox:             req.BBox,
		AvoidZoneIDs:     avoidIDs,
		Transport:        req.Transport,
	}

	stored, err := e.repo.Register(r.Context(), route)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(RegisterRouteResponse{
		Data: RegisterRouteResponseData{ActiveRoute: activeRouteToWire(stored)},
	}); err != nil {
		return
	}
}

// DeleteActiveRoute handles DELETE /api/routes/active/{active_route_id}.
func (e *ActiveRouteEngine) DeleteActiveRoute(w http.ResponseWriter, r *http.Request) {
	session := r.Header.Get("X-Volunteer-Session")
	if session == "" {
		http.Error(w, "X-Volunteer-Session header is required", http.StatusBadRequest)
		return
	}

	id := r.PathValue("active_route_id")
	if id == "" {
		http.Error(w, "active_route_id is required", http.StatusBadRequest)
		return
	}

	route, err := e.repo.GetByID(r.Context(), id)
	if err == ErrActiveRouteNotFound {
		http.Error(w, "active route not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if route.VolunteerSession != session {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := e.repo.Delete(r.Context(), id); err == ErrActiveRouteNotFound {
		http.Error(w, "active route not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if e.routeDeleted != nil {
		go e.routeDeleted(context.Background(), session)
	}

	w.WriteHeader(http.StatusNoContent)
}

// AcceptReroute handles POST /api/routes/active/{active_route_id}/accept-reroute.
// Recomputes ORS at accept time using the volunteer's latest reported progress,
// so the accepted route starts from the current position rather than the stale
// position at hazard-activation time.
func (e *ActiveRouteEngine) AcceptReroute(w http.ResponseWriter, r *http.Request) {
	session := r.Header.Get("X-Volunteer-Session")
	if session == "" {
		http.Error(w, "X-Volunteer-Session header is required", http.StatusBadRequest)
		return
	}

	id := r.PathValue("active_route_id")
	if id == "" {
		http.Error(w, "active_route_id is required", http.StatusBadRequest)
		return
	}

	var req AcceptRerouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := req.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate proposal (expired / not found checks; 410/404 semantics preserved).
	if _, err := e.repo.GetProposal(r.Context(), req.ProposalID); err != nil {
		switch err {
		case ErrProposalExpired:
			http.Error(w, "reroute proposal has expired", http.StatusGone)
		case ErrProposalNotFound:
			http.Error(w, "not found", http.StatusNotFound)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	// Get active route.
	activeRoute, err := e.repo.GetByID(r.Context(), id)
	if err == ErrActiveRouteNotFound {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Project current position from latest heartbeat progress.
	// Falls back to registered origin if demo store absent or session inactive.
	originLat, originLng := activeRoute.Origin.Lat, activeRoute.Origin.Lng
	currentProgress := 0.0
	if e.demoStore != nil {
		if progress, active, _ := e.demoStore.GetSessionProgress(r.Context(), session); active {
			originLat, originLng = projectPolylineAt(activeRoute.Geometry.Coordinates, progress)
			currentProgress = progress
		}
	}

	// Drop waypoints already visited so the reroute doesn't send the
	// volunteer back through pickups they've already made.
	pendingWaypoints := FilterPendingWaypoints(activeRoute.Geometry.Coordinates, activeRoute.Waypoints, currentProgress)

	// Build avoid_polygons for all current high+ zones (same logic as OnHazardActivated).
	var multiPoly [][][][2]float64
	if e.zoneRepo != nil {
		allZones, _ := e.zoneRepo.GetAll(r.Context())
		for _, z := range allZones {
			if z.Level != conditions_engine.ZoneLevelHigh && z.Level != conditions_engine.ZoneLevelActive {
				continue
			}
			ring := make([][2]float64, len(z.Polygon)+1)
			for i, p := range z.Polygon {
				ring[i] = [2]float64{p[1], p[0]}
			}
			if len(z.Polygon) > 0 {
				ring[len(z.Polygon)] = ring[0]
			}
			multiPoly = append(multiPoly, [][][2]float64{ring})
		}
	}

	// Build ORS coordinates from the filtered (pending) waypoint set.
	orsCoords := make([][2]float64, 0, 2+len(pendingWaypoints))
	orsCoords = append(orsCoords, [2]float64{originLng, originLat})
	for _, wp := range pendingWaypoints {
		orsCoords = append(orsCoords, [2]float64{wp.Lng, wp.Lat})
	}
	orsCoords = append(orsCoords, [2]float64{activeRoute.Destination.Lng, activeRoute.Destination.Lat})

	orsReq := route_engine.ORSDirectionsRequest{Coordinates: orsCoords}
	if len(multiPoly) > 0 {
		orsReq.Options = &route_engine.ORSOptions{
			AvoidPolygons: &route_engine.ORSAvoidPolygons{
				Type:        "MultiPolygon",
				Coordinates: multiPoly,
			},
		}
	}

	profile := route_engine.ProfileFromTransport(activeRoute.Transport)
	if profile == "" {
		profile = "driving-car"
	}

	// Recompute ORS at accept time.
	if e.ors == nil {
		http.Error(w, "reroute recomputation failed", http.StatusBadGateway)
		return
	}
	orsResp, err := e.ors.Directions(r.Context(), profile, orsReq)
	if err != nil {
		http.Error(w, "reroute recomputation failed", http.StatusBadGateway)
		return
	}
	newRoute := route_engine.BuildRouteData(orsResp)

	// Apply fresh route (marks proposal consumed; idempotent on second Accept).
	// Persisted Waypoints shrinks to the pending set so the next reroute filter
	// works against a smaller list.
	updated, err := e.repo.ApplyFreshRoute(r.Context(), id, req.ProposalID, newRoute, pendingWaypoints)
	if err == ErrActiveRouteNotFound {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Notify route-anchor-engine so its cached polyline tracks the new route.
	// Without this, post-reroute anchor projections would use the stale geometry
	// captured at session init. Fire-and-forget; failure should not block the
	// HTTP response.
	if e.rerouteAccepted != nil {
		e.rerouteAccepted(r.Context(), session, updated.Geometry)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(AcceptRerouteResponse{
		Data: AcceptRerouteResponseData{ActiveRoute: activeRouteToWire(updated)},
	}); err != nil {
		return
	}
}

// OnHazardActivated implements D5: for each active route, either emits
// reroute_proposal (new ORS detour found) or route_warning (inside hazard
// or ORS failed). Safe to call from any goroutine.
func (e *ActiveRouteEngine) OnHazardActivated(ctx context.Context, zone conditions_engine.Zone) {
	if e.hub == nil {
		return
	}
	routes, err := e.repo.GetAll(ctx)
	if err != nil || len(routes) == 0 {
		return
	}

	allZones, _ := e.zoneRepo.GetAll(ctx)

	for _, route := range routes {
		// Project current position from demo heartbeat (Bug A fix).
		// Falls back to registered origin if demo store is absent or session inactive.
		originLat, originLng := route.Origin.Lat, route.Origin.Lng
		currentProgress := 0.0
		if e.demoStore != nil {
			if progress, active, _ := e.demoStore.GetSessionProgress(ctx, route.VolunteerSession); active {
				originLat, originLng = projectPolylineAt(route.Geometry.Coordinates, progress)
				currentProgress = progress
			}
		}

		// D5: inside hazard polygon → route_warning
		poly := make([]route_engine.LatLng, len(zone.Polygon))
		for i, p := range zone.Polygon {
			poly[i] = route_engine.LatLng{Lat: p[0], Lng: p[1]}
		}
		if route_engine.PointInPolygon(originLat, originLng, poly) {
			e.sendRouteWarning(ctx, route, zone)
			continue
		}

		// D5: skip ORS entirely if hazard polygon does not intersect this route.
		if !PolylineIntersectsPolygon(route.Geometry.Coordinates, zone.Polygon) {
			continue
		}

		// Build avoid_polygons for all current high+ zones
		var multiPoly [][][][2]float64
		for _, z := range allZones {
			if z.Level != conditions_engine.ZoneLevelHigh && z.Level != conditions_engine.ZoneLevelActive {
				continue
			}
			ring := make([][2]float64, len(z.Polygon)+1)
			for i, p := range z.Polygon {
				ring[i] = [2]float64{p[1], p[0]} // Leaflet [lat,lng] → GeoJSON [lng,lat]
			}
			if len(z.Polygon) > 0 {
				ring[len(z.Polygon)] = ring[0]
			}
			multiPoly = append(multiPoly, [][][2]float64{ring})
		}

		// Build ORS request from the filtered (pending) waypoint set so the
		// proposed detour doesn't backtrack to already-visited pickups.
		pendingWaypoints := FilterPendingWaypoints(route.Geometry.Coordinates, route.Waypoints, currentProgress)
		orsCoords := make([][2]float64, 0, 2+len(pendingWaypoints))
		orsCoords = append(orsCoords, [2]float64{originLng, originLat})
		for _, wp := range pendingWaypoints {
			orsCoords = append(orsCoords, [2]float64{wp.Lng, wp.Lat})
		}
		orsCoords = append(orsCoords, [2]float64{route.Destination.Lng, route.Destination.Lat})

		orsReq := route_engine.ORSDirectionsRequest{Coordinates: orsCoords}
		if len(multiPoly) > 0 {
			orsReq.Options = &route_engine.ORSOptions{
				AvoidPolygons: &route_engine.ORSAvoidPolygons{
					Type:        "MultiPolygon",
					Coordinates: multiPoly,
				},
			}
		}

		profile := route_engine.ProfileFromTransport(route.Transport)
		if profile == "" {
			profile = "driving-car" // safe default
		}

		// ORS call — any error → route_warning (D5)
		if e.ors == nil {
			e.sendRouteWarning(ctx, route, zone)
			continue
		}
		orsResp, err := e.ors.Directions(ctx, profile, orsReq)
		if err != nil {
			e.sendRouteWarning(ctx, route, zone)
			continue
		}

		// Build proposal
		newRoute := route_engine.BuildRouteData(orsResp)
		etaDelta := newRoute.DurationSeconds - route.DurationSeconds
		if math.Abs(etaDelta) < 5 {
			// Hazard does not meaningfully affect this route; suppress silently.
			continue
		}
		proposalID := newProposalID()
		proposal := RerouteProposal{
			ID:               proposalID,
			ActiveRouteID:    route.ID,
			TriggeringZoneID: zone.ID,
			NewRoute:         newRoute,
			EtaDeltaSeconds:  etaDelta,
			CreatedAt:        time.Now(),
		}
		if err := e.repo.StoreProposal(ctx, proposal); err != nil {
			e.sendRouteWarning(ctx, route, zone)
			continue
		}

		e.sendRerouteProposal(ctx, route, zone, proposal)
	}
}

func (e *ActiveRouteEngine) sendRouteWarning(_ context.Context, route ActiveRoute, zone conditions_engine.Zone) {
	envelope := conditions_engine.WsEnvelopeWire{
		Type: "route_warning",
		Ts:   time.Now().UTC().Format(time.RFC3339),
		Payload: RouteWarningPayloadWire{
			ActiveRouteID:    route.ID,
			TriggeringZoneID: zone.ID,
			Message:          "Your route is affected by a hazard. Please proceed with caution.",
		},
	}
	_ = e.hub.SendToClient(route.VolunteerSession, envelope) // disconnected client → silently ignore
}

func (e *ActiveRouteEngine) sendRerouteProposal(_ context.Context, route ActiveRoute, zone conditions_engine.Zone, proposal RerouteProposal) {
	envelope := conditions_engine.WsEnvelopeWire{
		Type: "reroute_proposal",
		Ts:   time.Now().UTC().Format(time.RFC3339),
		Payload: RerouteProposalPayloadWire{
			ProposalID:       proposal.ID,
			ActiveRouteID:    route.ID,
			TriggeringZoneID: zone.ID,
			EtaDeltaSeconds:  proposal.EtaDeltaSeconds,
			Route:            proposal.NewRoute,
		},
	}
	_ = e.hub.SendToClient(route.VolunteerSession, envelope)
}

// projectPolylineAt returns the (lat, lng) at the given fractional progress
// along a GeoJSON coordinate sequence (each element is [lng, lat]).
// Returns origin (coords[0]) on empty input or progress <= 0;
// returns last point on progress >= 1.
func projectPolylineAt(coords [][]float64, progress float64) (lat, lng float64) {
	n := len(coords)
	if n == 0 {
		return 0, 0
	}
	if n == 1 {
		return coords[0][1], coords[0][0]
	}
	haversineM := func(lat1, lng1, lat2, lng2 float64) float64 {
		const R = 6_371_000.0
		dlat := (lat2 - lat1) * math.Pi / 180
		dlng := (lng2 - lng1) * math.Pi / 180
		sLat := math.Sin(dlat / 2)
		sLng := math.Sin(dlng / 2)
		h := sLat*sLat + math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*sLng*sLng
		return R * 2 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
	}
	segLens := make([]float64, n-1)
	total := 0.0
	for i := 0; i < n-1; i++ {
		segLens[i] = haversineM(coords[i][1], coords[i][0], coords[i+1][1], coords[i+1][0])
		total += segLens[i]
	}
	if total == 0 || progress <= 0 {
		return coords[0][1], coords[0][0]
	}
	if progress >= 1 {
		return coords[n-1][1], coords[n-1][0]
	}
	target := progress * total
	cum := 0.0
	for i := 0; i < n-1; i++ {
		if cum+segLens[i] >= target || i == n-2 {
			frac := 0.0
			if segLens[i] > 0 {
				frac = (target - cum) / segLens[i]
			}
			return coords[i][1] + frac*(coords[i+1][1]-coords[i][1]),
				coords[i][0] + frac*(coords[i+1][0]-coords[i][0])
		}
		cum += segLens[i]
	}
	return coords[n-1][1], coords[n-1][0]
}

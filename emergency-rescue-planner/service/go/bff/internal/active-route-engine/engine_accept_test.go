package active_route_engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	route_engine "bff/internal/route-engine"
)

// --- Mock DemoSessionStore ---

type mockDemoStore struct {
	progress float64
	active   bool
}

func (m *mockDemoStore) Upsert(_ context.Context, _ string, _ float64, _ bool) error {
	return nil
}

func (m *mockDemoStore) GetSessionProgress(_ context.Context, _ string) (float64, bool, error) {
	return m.progress, m.active, nil
}

func (m *mockDemoStore) GetSessionAutoExpand(_ context.Context, _ string) (bool, bool, error) {
	return false, m.active, nil
}

// --- Mock orsDirectioner backed by httptest.Server ---

// testORSClient satisfies orsDirectioner by hitting a real httptest.Server.
type testORSClient struct {
	serverURL  string
	httpClient *http.Client
}

func (c *testORSClient) Directions(ctx context.Context, profile string, req route_engine.ORSDirectionsRequest) (*route_engine.ORSDirectionsResponse, error) {
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.serverURL+"/v2/directions/"+profile+"/geojson",
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ORS request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ORS %d", resp.StatusCode)
	}
	var orsResp route_engine.ORSDirectionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&orsResp); err != nil {
		return nil, fmt.Errorf("decode ORS: %w", err)
	}
	if len(orsResp.Features) == 0 {
		return nil, fmt.Errorf("ORS returned empty feature collection")
	}
	return &orsResp, nil
}

// --- Minimal ORS success response ---

func minimalORSResponse() route_engine.ORSDirectionsResponse {
	return route_engine.ORSDirectionsResponse{
		Features: []route_engine.ORSFeature{
			{
				Geometry: route_engine.GeoJSONGeometry{
					Type:        "LineString",
					Coordinates: [][]float64{{144.96, -37.81}, {144.97, -37.82}},
				},
				Properties: route_engine.ORSRouteProperties{
					Summary: route_engine.ORSRouteSummary{
						Duration: 600,
						Distance: 5000,
					},
				},
			},
		},
		BBox: []float64{144.96, -37.82, 144.97, -37.81},
	}
}

// --- Test helper: build a seeded engine with one registered route and one proposal ---

type testFixture struct {
	engine  *ActiveRouteEngine
	repo    *MockActiveRouteRepository
	routeID string
	propID  string
}

func buildFixture(t *testing.T, ors orsDirectioner, demo *mockDemoStore) testFixture {
	t.Helper()

	repo := NewMockActiveRouteRepository()

	route := ActiveRoute{
		VolunteerSession: "sess-1",
		Origin:           LatLng{Lat: -37.81, Lng: 144.96},
		Destination:      LatLng{Lat: -37.90, Lng: 145.00},
		Waypoints:        []LatLng{},
		Geometry: route_engine.GeoJSONGeometry{
			Type:        "LineString",
			Coordinates: [][]float64{{144.96, -37.81}, {144.98, -37.85}, {145.00, -37.90}},
		},
		DurationSeconds: 1200,
		DistanceMetres:  10000,
		Transport:       "drive",
	}
	stored, _ := repo.Register(context.Background(), route)

	proposal := RerouteProposal{
		ID:               "prop-test-1",
		ActiveRouteID:    stored.ID,
		TriggeringZoneID: "rz1",
		NewRoute: route_engine.RouteData{
			Geometry:        route_engine.GeoJSONGeometry{Type: "LineString"},
			DurationSeconds: 900,
			DistanceMetres:  8000,
		},
		EtaDeltaSeconds: -300,
		CreatedAt:       time.Now(),
	}
	_ = repo.StoreProposal(context.Background(), proposal)

	eng := NewActiveRouteEngine(repo)
	eng.ors = ors
	if demo != nil {
		eng.demoStore = demo
	}

	return testFixture{engine: eng, repo: repo, routeID: stored.ID, propID: proposal.ID}
}

// --- Recording ORS client (for waypoint-filter test) ---

// recordingORSClient captures the last ORS request without hitting an HTTP
// server. Returns a configurable response.
type recordingORSClient struct {
	lastReq route_engine.ORSDirectionsRequest
	resp    route_engine.ORSDirectionsResponse
}

func (c *recordingORSClient) Directions(_ context.Context, _ string, req route_engine.ORSDirectionsRequest) (*route_engine.ORSDirectionsResponse, error) {
	c.lastReq = req
	r := c.resp
	return &r, nil
}

// demoStore active with progress=0.5 → ORS called, route updated.
func TestAcceptReroute_NormalPath_WithProgress(t *testing.T) {
	orsResp := minimalORSResponse()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(orsResp)
	}))
	defer srv.Close()

	ors := &testORSClient{serverURL: srv.URL, httpClient: &http.Client{}}
	demo := &mockDemoStore{progress: 0.5, active: true}
	fx := buildFixture(t, ors, demo)

	body := `{"proposal_id":"prop-test-1"}`
	req := httptest.NewRequest("POST", "/api/routes/active/"+fx.routeID+"/accept-reroute", strings.NewReader(body))
	req.Header.Set("X-Volunteer-Session", "sess-1")
	req.SetPathValue("active_route_id", fx.routeID)
	rr := httptest.NewRecorder()

	fx.engine.AcceptReroute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp AcceptRerouteResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// Route geometry must be updated from ORS response (not the original 1200 s).
	if resp.Data.ActiveRoute.DurationSeconds != 600 {
		t.Errorf("expected duration_seconds=600 from ORS, got %v", resp.Data.ActiveRoute.DurationSeconds)
	}
}

// demoStore inactive → fallback to registered origin → ORS called → 200.
func TestAcceptReroute_DemoStoreInactive_FallbackOrigin(t *testing.T) {
	orsResp := minimalORSResponse()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(orsResp)
	}))
	defer srv.Close()

	ors := &testORSClient{serverURL: srv.URL, httpClient: &http.Client{}}
	demo := &mockDemoStore{progress: 0, active: false} // inactive
	fx := buildFixture(t, ors, demo)

	body := `{"proposal_id":"prop-test-1"}`
	req := httptest.NewRequest("POST", "/api/routes/active/"+fx.routeID+"/accept-reroute", strings.NewReader(body))
	req.Header.Set("X-Volunteer-Session", "sess-1")
	req.SetPathValue("active_route_id", fx.routeID)
	rr := httptest.NewRecorder()

	fx.engine.AcceptReroute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp AcceptRerouteResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.ActiveRoute.DurationSeconds != 600 {
		t.Errorf("expected 600 from ORS, got %v", resp.Data.ActiveRoute.DurationSeconds)
	}
}

// ORS returns 500 → handler returns 502, route geometry unchanged.
func TestAcceptReroute_ORSError_Returns502(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "ORS upstream error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	ors := &testORSClient{serverURL: srv.URL, httpClient: &http.Client{}}
	fx := buildFixture(t, ors, nil)

	body := `{"proposal_id":"prop-test-1"}`
	req := httptest.NewRequest("POST", "/api/routes/active/"+fx.routeID+"/accept-reroute", strings.NewReader(body))
	req.Header.Set("X-Volunteer-Session", "sess-1")
	req.SetPathValue("active_route_id", fx.routeID)
	rr := httptest.NewRecorder()

	fx.engine.AcceptReroute(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rr.Code, rr.Body.String())
	}

	// Route geometry must not have changed.
	stored, _ := fx.repo.GetByID(context.Background(), fx.routeID)
	if stored.DurationSeconds != 1200 {
		t.Errorf("route should be unchanged after ORS failure, got %v", stored.DurationSeconds)
	}
}

// Proposal expired → 410 Gone, ORS not called.
func TestAcceptReroute_ProposalExpired_Returns410(t *testing.T) {
	orsCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orsCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ors := &testORSClient{serverURL: srv.URL, httpClient: &http.Client{}}
	repo := NewMockActiveRouteRepository()

	route := ActiveRoute{
		VolunteerSession: "sess-exp",
		Origin:           LatLng{Lat: -37.81, Lng: 144.96},
		Destination:      LatLng{Lat: -37.90, Lng: 145.00},
		Waypoints:        []LatLng{},
		Geometry:         route_engine.GeoJSONGeometry{Type: "LineString", Coordinates: [][]float64{{144.96, -37.81}}},
		DurationSeconds:  1200,
		Transport:        "drive",
	}
	stored, _ := repo.Register(context.Background(), route)

	// Store an already-expired proposal (CreatedAt 10 minutes ago).
	expiredProposal := RerouteProposal{
		ID:            "prop-expired",
		ActiveRouteID: stored.ID,
		NewRoute:      route_engine.RouteData{},
		CreatedAt:     time.Now().Add(-10 * time.Minute),
	}
	_ = repo.StoreProposal(context.Background(), expiredProposal)

	eng := NewActiveRouteEngine(repo)
	eng.ors = ors

	body := `{"proposal_id":"prop-expired"}`
	req := httptest.NewRequest("POST", "/api/routes/active/"+stored.ID+"/accept-reroute", strings.NewReader(body))
	req.Header.Set("X-Volunteer-Session", "sess-exp")
	req.SetPathValue("active_route_id", stored.ID)
	rr := httptest.NewRecorder()

	eng.AcceptReroute(rr, req)

	if rr.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d: %s", rr.Code, rr.Body.String())
	}
	if orsCalled {
		t.Error("ORS must not be called when proposal is expired")
	}
}

// Proposal not found → 404, ORS not called.
func TestAcceptReroute_ProposalNotFound_Returns404(t *testing.T) {
	orsCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orsCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ors := &testORSClient{serverURL: srv.URL, httpClient: &http.Client{}}
	repo := NewMockActiveRouteRepository()

	route := ActiveRoute{
		VolunteerSession: "sess-nf",
		Origin:           LatLng{Lat: -37.81, Lng: 144.96},
		Destination:      LatLng{Lat: -37.90, Lng: 145.00},
		Waypoints:        []LatLng{},
		Geometry:         route_engine.GeoJSONGeometry{Type: "LineString", Coordinates: [][]float64{{144.96, -37.81}}},
		DurationSeconds:  1200,
		Transport:        "drive",
	}
	stored, _ := repo.Register(context.Background(), route)

	eng := NewActiveRouteEngine(repo)
	eng.ors = ors

	body := `{"proposal_id":"prop-does-not-exist"}`
	req := httptest.NewRequest("POST", "/api/routes/active/"+stored.ID+"/accept-reroute", strings.NewReader(body))
	req.Header.Set("X-Volunteer-Session", "sess-nf")
	req.SetPathValue("active_route_id", stored.ID)
	rr := httptest.NewRecorder()

	eng.AcceptReroute(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
	if orsCalled {
		t.Error("ORS must not be called when proposal is not found")
	}
}

// AcceptReroute must filter out already-visited waypoints so the recomputed
// route does not backtrack through completed pickups. Fixture: 4-vertex
// constant-lat polyline (progress markers at 0, 1/3, 2/3, 1) with waypoints
// at the three non-start vertices; at currentProgress=0.5 only wp@2/3 and
// wp@1.0 survive the filter.
func TestAcceptReroute_FiltersPassedWaypoints(t *testing.T) {
	repo := NewMockActiveRouteRepository()
	polyline := [][]float64{
		{144.96, -37.81},
		{144.97, -37.81}, // wp@1/3 → will be filtered out at progress=0.5
		{144.98, -37.81}, // wp@2/3 → survives
		{144.99, -37.81}, // wp@1.0 (= destination), kept separate
	}
	route := ActiveRoute{
		VolunteerSession: "sess-multi",
		Origin:           LatLng{Lat: -37.81, Lng: 144.96},
		Destination:      LatLng{Lat: -37.81, Lng: 144.99},
		Waypoints: []LatLng{
			{Lat: -37.81, Lng: 144.97}, // already passed at progress=0.5
			{Lat: -37.81, Lng: 144.98}, // still ahead
		},
		Geometry: route_engine.GeoJSONGeometry{
			Type:        "LineString",
			Coordinates: polyline,
		},
		DurationSeconds: 1000,
		DistanceMetres:  3000,
		Transport:       "drive",
	}
	stored, _ := repo.Register(context.Background(), route)

	proposal := RerouteProposal{
		ID:               "prop-multi",
		ActiveRouteID:    stored.ID,
		TriggeringZoneID: "rz",
		NewRoute: route_engine.RouteData{
			Geometry: route_engine.GeoJSONGeometry{Type: "LineString"},
		},
		CreatedAt: time.Now(),
	}
	_ = repo.StoreProposal(context.Background(), proposal)

	rec := &recordingORSClient{
		resp: route_engine.ORSDirectionsResponse{
			Features: []route_engine.ORSFeature{{
				Geometry: route_engine.GeoJSONGeometry{
					Type:        "LineString",
					Coordinates: [][]float64{{144.97, -37.81}, {144.99, -37.81}},
				},
				Properties: route_engine.ORSRouteProperties{
					Summary: route_engine.ORSRouteSummary{Duration: 800, Distance: 2000},
				},
			}},
		},
	}

	eng := NewActiveRouteEngine(repo)
	eng.ors = rec
	eng.demoStore = &mockDemoStore{progress: 0.5, active: true}

	body := `{"proposal_id":"prop-multi"}`
	req := httptest.NewRequest("POST", "/api/routes/active/"+stored.ID+"/accept-reroute", strings.NewReader(body))
	req.Header.Set("X-Volunteer-Session", "sess-multi")
	req.SetPathValue("active_route_id", stored.ID)
	rr := httptest.NewRecorder()

	eng.AcceptReroute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// ORS request must have exactly [currentPos, wp@2/3, destination] — wp@1/3
	// (already passed) must NOT appear.
	coords := rec.lastReq.Coordinates
	if len(coords) != 3 {
		t.Fatalf("expected 3 ORS coordinates (origin + 1 pending wp + dest), got %d: %v", len(coords), coords)
	}
	// coord[0] is currentPos projected at progress=0.5 → polyline midpoint ≈ 144.975
	// coord[1] must be the wp@2/3, i.e. lng ≈ 144.98
	if math.Abs(coords[1][0]-144.98) > 1e-6 {
		t.Errorf("ORS coordinate[1] (sole pending waypoint) lng=%v, expected 144.98", coords[1][0])
	}
	// coord[2] is destination, lng ≈ 144.99
	if math.Abs(coords[2][0]-144.99) > 1e-6 {
		t.Errorf("ORS coordinate[2] (destination) lng=%v, expected 144.99", coords[2][0])
	}

	// Persisted ActiveRoute.Waypoints (internal domain state — not in the
	// HTTP wire response) should shrink to the single survivor so the next
	// reroute filter works against a smaller list.
	persisted, err := repo.GetByID(context.Background(), stored.ID)
	if err != nil {
		t.Fatalf("fetch persisted route: %v", err)
	}
	if len(persisted.Waypoints) != 1 {
		t.Errorf("expected persisted Waypoints len=1, got %d: %v", len(persisted.Waypoints), persisted.Waypoints)
	} else if math.Abs(persisted.Waypoints[0].Lng-144.98) > 1e-6 {
		t.Errorf("persisted survivor lng=%v, expected 144.98", persisted.Waypoints[0].Lng)
	}
}

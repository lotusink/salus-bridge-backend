package active_route_engine

import (
	"math"
	"testing"
)

func TestPolylineIntersectsPolygon(t *testing.T) {
	cases := []struct {
		name     string
		polyline [][]float64
		polygon  [][2]float64
		want     bool
	}{
		{
			name:     "case1 route vertex inside polygon",
			polyline: [][]float64{{144.960, -37.810}, {144.965, -37.815}},
			polygon:  [][2]float64{{-37.808, 144.958}, {-37.808, 144.968}, {-37.820, 144.968}, {-37.820, 144.958}},
			want:     true,
		},
		{
			name:     "case2 route crosses polygon",
			polyline: [][]float64{{144.950, -37.810}, {144.975, -37.810}},
			polygon:  [][2]float64{{-37.808, 144.960}, {-37.808, 144.965}, {-37.815, 144.965}, {-37.815, 144.960}},
			want:     true,
		},
		{
			name:     "case3 route outside bbox lat no overlap",
			polyline: [][]float64{{144.950, -37.800}, {144.960, -37.800}},
			polygon:  [][2]float64{{-37.820, 144.958}, {-37.820, 144.968}, {-37.830, 144.968}, {-37.830, 144.958}},
			want:     false,
		},
		{
			name:     "case4 bbox no overlap quick-reject",
			polyline: [][]float64{{145.100, -37.810}, {145.200, -37.810}},
			polygon:  [][2]float64{{-37.808, 144.958}, {-37.808, 144.968}, {-37.815, 144.968}, {-37.815, 144.958}},
			want:     false,
		},
		{
			name:     "case5 polygon entirely north of polyline",
			polyline: [][]float64{{144.950, -37.850}, {144.990, -37.850}},
			polygon:  [][2]float64{{-37.808, 144.960}, {-37.808, 144.965}, {-37.812, 144.965}, {-37.812, 144.960}},
			want:     false,
		},
		{
			name:     "case6 collinear polygon vertex on polyline",
			polyline: [][]float64{{144.960, -37.810}, {144.970, -37.810}},
			polygon:  [][2]float64{{-37.810, 144.965}, {-37.805, 144.965}, {-37.805, 144.970}, {-37.810, 144.970}},
			want:     true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PolylineIntersectsPolygon(tc.polyline, tc.polygon)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// =============================================================================
// Waypoint completion filtering
// =============================================================================

// Test polyline: 4 vertices along constant latitude. 3 equal-length segments
// so vertex i sits at progress fraction i/3.
// GeoJSON order [lng, lat].
var testPolyline = [][]float64{
	{144.96, -37.81}, // i=0, progress 0
	{144.97, -37.81}, // i=1, progress 1/3
	{144.98, -37.81}, // i=2, progress 2/3
	{144.99, -37.81}, // i=3, progress 1
}

func TestPolylineProgressForPoint_StartEndMid(t *testing.T) {
	cases := []struct {
		name    string
		lat     float64
		lng     float64
		wantMin float64
		wantMax float64
	}{
		{"start vertex", -37.81, 144.96, -1e-6, 1e-6},
		{"end vertex", -37.81, 144.99, 1 - 1e-6, 1 + 1e-6},
		{"mid vertex (2/3)", -37.81, 144.98, 2.0/3 - 1e-6, 2.0/3 + 1e-6},
		{"first quarter (snaps to vertex 1 at 1/3)", -37.811, 144.971, 1.0/3 - 1e-6, 1.0/3 + 1e-6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PolylineProgressForPoint(testPolyline, tc.lat, tc.lng)
			if got < tc.wantMin || got > tc.wantMax {
				t.Errorf("progress=%v, want in [%v, %v]", got, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestPolylineProgressForPoint_DegenerateInputs(t *testing.T) {
	// Fewer than 2 vertices → 0
	if got := PolylineProgressForPoint([][]float64{{144.96, -37.81}}, -37.81, 144.96); got != 0 {
		t.Errorf("single-vertex polyline: want 0, got %v", got)
	}
	if got := PolylineProgressForPoint(nil, -37.81, 144.96); got != 0 {
		t.Errorf("nil polyline: want 0, got %v", got)
	}
}

func TestFilterPendingWaypoints_DropsBehind(t *testing.T) {
	// Three waypoints positioned at vertices 1, 2, 3 → projected progress 1/3, 2/3, 1.
	waypoints := []LatLng{
		{Lat: -37.81, Lng: 144.97}, // progress 1/3
		{Lat: -37.81, Lng: 144.98}, // progress 2/3
		{Lat: -37.81, Lng: 144.99}, // progress 1
	}
	// currentProgress=0.5 + slop 0.01 = 0.51
	//   wp@1/3 (~0.333) NOT > 0.51 → drop
	//   wp@2/3 (~0.667) > 0.51 → keep
	//   wp@1.0 > 0.51 → keep
	got := FilterPendingWaypoints(testPolyline, waypoints, 0.5)
	if len(got) != 2 {
		t.Fatalf("expected 2 pending, got %d (%v)", len(got), got)
	}
	if !nearLng(got[0].Lng, 144.98) || !nearLng(got[1].Lng, 144.99) {
		t.Errorf("expected [144.98, 144.99] survivors, got %v", got)
	}
}

func TestFilterPendingWaypoints_AllAhead(t *testing.T) {
	waypoints := []LatLng{
		{Lat: -37.81, Lng: 144.97},
		{Lat: -37.81, Lng: 144.98},
	}
	got := FilterPendingWaypoints(testPolyline, waypoints, 0.0)
	if len(got) != 2 {
		t.Errorf("expected all 2 retained, got %d", len(got))
	}
}

func TestFilterPendingWaypoints_AllBehind(t *testing.T) {
	waypoints := []LatLng{
		{Lat: -37.81, Lng: 144.97},
		{Lat: -37.81, Lng: 144.98},
	}
	got := FilterPendingWaypoints(testPolyline, waypoints, 0.99)
	if len(got) != 0 {
		t.Errorf("expected 0 pending at progress 0.99, got %d (%v)", len(got), got)
	}
}

func TestFilterPendingWaypoints_AtBoundary(t *testing.T) {
	// wp at progress 2/3 ≈ 0.6667. currentProgress = 0.6667 → 0.6667 > 0.6767 is
	// false → wp dropped. Slop ensures "just passed" waypoints don't refire.
	waypoints := []LatLng{{Lat: -37.81, Lng: 144.98}}
	got := FilterPendingWaypoints(testPolyline, waypoints, 2.0/3)
	if len(got) != 0 {
		t.Errorf("at-boundary: expected wp dropped, got %d (%v)", len(got), got)
	}
}

// nearLng accepts small float roundoff in lng comparisons.
func nearLng(got, want float64) bool { return math.Abs(got-want) < 1e-6 }

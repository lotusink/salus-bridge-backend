package active_route_engine

import (
	"math"

	route_engine "bff/internal/route-engine"
)

// PolylineIntersectsPolygon returns true when the route polyline touches or
// crosses the hazard polygon. Used by OnHazardActivated to skip ORS calls
// for hazards that do not affect the route.
//
// polyline: GeoJSON order [lng, lat] — route.Geometry.Coordinates
// polygon:  Leaflet order [[lat, lng], ...] — zone.Polygon
//
// Returns false when either input has fewer than 2 points.
func PolylineIntersectsPolygon(polyline [][]float64, polygon [][2]float64) bool {
	if len(polyline) < 2 || len(polygon) < 2 {
		return false
	}

	// 1. BBox quick-reject — O(N+M), eliminates most distant hazards instantly.
	if !bboxesOverlap(polyline, polygon) {
		return false
	}

	// 2. Any polyline vertex inside polygon?
	poly := make([]route_engine.LatLng, len(polygon))
	for i, p := range polygon {
		poly[i] = route_engine.LatLng{Lat: p[0], Lng: p[1]}
	}
	for _, c := range polyline {
		// c is [lng, lat]; PointInPolygon expects (lat, lng).
		if route_engine.PointInPolygon(c[1], c[0], poly) {
			return true
		}
	}

	// 3. Any polyline segment intersects any polygon edge?
	// All coords normalised to (lng, lat) before passing to segmentsIntersect.
	for i := 0; i < len(polyline)-1; i++ {
		ax, ay := polyline[i][0], polyline[i][1] // [lng, lat]
		bx, by := polyline[i+1][0], polyline[i+1][1]
		for j := 0; j < len(polygon); j++ {
			c := polygon[j]
			d := polygon[(j+1)%len(polygon)]
			// polygon is [lat, lng]; swap to (lng, lat) for consistent coords.
			cx, cy := c[1], c[0]
			dx, dy := d[1], d[0]
			if segmentsIntersect(ax, ay, bx, by, cx, cy, dx, dy) {
				return true
			}
		}
	}
	return false
}

// bboxesOverlap returns true when the bounding boxes of polyline and polygon overlap.
// polyline uses [lng, lat] order; polygon uses [lat, lng] order.
func bboxesOverlap(polyline [][]float64, polygon [][2]float64) bool {
	plMinLng := polyline[0][0]
	plMaxLng := polyline[0][0]
	plMinLat := polyline[0][1]
	plMaxLat := polyline[0][1]
	for _, c := range polyline {
		plMinLng = math.Min(plMinLng, c[0])
		plMaxLng = math.Max(plMaxLng, c[0])
		plMinLat = math.Min(plMinLat, c[1])
		plMaxLat = math.Max(plMaxLat, c[1])
	}

	pgMinLat := polygon[0][0]
	pgMaxLat := polygon[0][0]
	pgMinLng := polygon[0][1]
	pgMaxLng := polygon[0][1]
	for _, p := range polygon {
		pgMinLat = math.Min(pgMinLat, p[0])
		pgMaxLat = math.Max(pgMaxLat, p[0])
		pgMinLng = math.Min(pgMinLng, p[1])
		pgMaxLng = math.Max(pgMaxLng, p[1])
	}

	return plMaxLng >= pgMinLng && plMinLng <= pgMaxLng &&
		plMaxLat >= pgMinLat && plMinLat <= pgMaxLat
}

// segmentsIntersect returns true when segment AB intersects segment CD.
// All parameters are (lng, lat) order. Collinear overlap is treated as
// intersection (conservative — avoids false negatives at polygon edges).
//
// Algorithm: orientation-based (CLRS §33.1 / Shamos-Hoey).
func segmentsIntersect(ax, ay, bx, by, cx, cy, dx, dy float64) bool {
	d1 := cross(cx, cy, dx, dy, ax, ay)
	d2 := cross(cx, cy, dx, dy, bx, by)
	d3 := cross(ax, ay, bx, by, cx, cy)
	d4 := cross(ax, ay, bx, by, dx, dy)

	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}

	// Collinear cases: point lies on the other segment.
	if d1 == 0 && onSeg(cx, cy, dx, dy, ax, ay) {
		return true
	}
	if d2 == 0 && onSeg(cx, cy, dx, dy, bx, by) {
		return true
	}
	if d3 == 0 && onSeg(ax, ay, bx, by, cx, cy) {
		return true
	}
	if d4 == 0 && onSeg(ax, ay, bx, by, dx, dy) {
		return true
	}
	return false
}

// cross returns the 2D cross product of vectors (P→Q) and (P→R).
func cross(px, py, qx, qy, rx, ry float64) float64 {
	return (qx-px)*(ry-py) - (qy-py)*(rx-px)
}

// onSeg returns true when point R lies on segment PQ (all collinear, caller ensures).
func onSeg(px, py, qx, qy, rx, ry float64) bool {
	return rx >= math.Min(px, qx) && rx <= math.Max(px, qx) &&
		ry >= math.Min(py, qy) && ry <= math.Max(py, qy)
}

// waypointPassedSlop is the cushion subtracted from a waypoint's progress
// before comparing against currentProgress. 1% tolerates float roundoff and
// the snap distance between a waypoint's recorded lat/lng and the polyline
// vertex it maps to. Effect: a waypoint at the user's exact current progress
// is treated as "passed" rather than "pending".
const waypointPassedSlop = 0.01

// PolylineProgressForPoint returns the cumulative-length fraction at which
// the point (lat, lng) projects onto the polyline. Uses nearest-vertex
// search — since ORS routes through waypoints, each waypoint coincides with
// a polyline vertex within road-snap tolerance.
//
// polyline: GeoJSON [lng, lat] order.
// Returns 0 when polyline has < 2 vertices or zero total length.
func PolylineProgressForPoint(polyline [][]float64, lat, lng float64) float64 {
	if len(polyline) < 2 {
		return 0
	}
	cum := make([]float64, len(polyline))
	cum[0] = 0
	for i := 1; i < len(polyline); i++ {
		cum[i] = cum[i-1] + haversineMeters(
			polyline[i-1][1], polyline[i-1][0],
			polyline[i][1], polyline[i][0],
		)
	}
	total := cum[len(cum)-1]
	if total == 0 {
		return 0
	}
	bestIdx := 0
	bestDist := math.MaxFloat64
	for i, c := range polyline {
		d := haversineMeters(c[1], c[0], lat, lng)
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	return cum[bestIdx] / total
}

// FilterPendingWaypoints returns the subset of waypoints whose projected
// progress on the polyline lies strictly ahead of currentProgress + slop.
// Used by AcceptReroute and OnHazardActivated to avoid feeding ORS a
// waypoint the volunteer has already visited, which would generate a
// backtracking route.
func FilterPendingWaypoints(polyline [][]float64, waypoints []LatLng, currentProgress float64) []LatLng {
	out := make([]LatLng, 0, len(waypoints))
	threshold := currentProgress + waypointPassedSlop
	for _, wp := range waypoints {
		if PolylineProgressForPoint(polyline, wp.Lat, wp.Lng) > threshold {
			out = append(out, wp)
		}
	}
	return out
}

// haversineMeters computes great-circle distance between two lat/lng points,
// in metres. Local to this package; the route-anchor-engine has its own
// equivalent and we keep them independent to avoid a circular import.
func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusM = 6_371_000.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

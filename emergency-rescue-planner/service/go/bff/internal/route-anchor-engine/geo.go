package route_anchor_engine

import "math"

func haversineM(a, b LatLng) float64 {
	const R = 6_371_000.0
	dlat := (b.Lat - a.Lat) * math.Pi / 180
	dlng := (b.Lng - a.Lng) * math.Pi / 180
	sinLat := math.Sin(dlat / 2)
	sinLng := math.Sin(dlng / 2)
	h := sinLat*sinLat + math.Cos(a.Lat*math.Pi/180)*math.Cos(b.Lat*math.Pi/180)*sinLng*sinLng
	return R * 2 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
}

func computeBearing(from, to LatLng) Direction {
	lat1 := from.Lat * math.Pi / 180
	lat2 := to.Lat * math.Pi / 180
	dLng := (to.Lng - from.Lng) * math.Pi / 180
	y := math.Sin(dLng) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLng)
	deg := math.Atan2(y, x) * 180 / math.Pi
	return Direction(math.Mod(deg+360, 360))
}

// ProjectAnchorToLatLng walks polyline and returns the point and travel bearing at progress.
//
// Corner cases:
//   - len(polyline) == 0 → returns LatLng{}, Direction(0). Caller must ensure non-empty polyline.
//   - len(polyline) == 1 → returns polyline[0], Direction(0).
//   - progress <= 0 → returns polyline[0], bearing of first segment (or 0 if single point).
//   - progress >= 1 → returns polyline[n-1], bearing of last segment.
func ProjectAnchorToLatLng(polyline []LatLng, progress float64) (LatLng, Direction) {
	n := len(polyline)
	if n == 0 {
		return LatLng{}, Direction(0)
	}
	if n == 1 {
		return polyline[0], Direction(0)
	}

	segLens := make([]float64, n-1)
	total := 0.0
	for i := 0; i < n-1; i++ {
		segLens[i] = haversineM(polyline[i], polyline[i+1])
		total += segLens[i]
	}
	if total == 0 {
		return polyline[0], Direction(0)
	}

	if progress <= 0 {
		return polyline[0], computeBearing(polyline[0], polyline[1])
	}
	if progress >= 1 {
		return polyline[n-1], computeBearing(polyline[n-2], polyline[n-1])
	}

	target := progress * total
	cum := 0.0
	for i := 0; i < n-1; i++ {
		if cum+segLens[i] >= target || i == n-2 {
			frac := 0.0
			if segLens[i] > 0 {
				frac = (target - cum) / segLens[i]
			}
			pt := LatLng{
				Lat: polyline[i].Lat + frac*(polyline[i+1].Lat-polyline[i].Lat),
				Lng: polyline[i].Lng + frac*(polyline[i+1].Lng-polyline[i].Lng),
			}
			return pt, computeBearing(polyline[i], polyline[i+1])
		}
		cum += segLens[i]
	}
	return polyline[n-1], computeBearing(polyline[n-2], polyline[n-1])
}

// ApplyPerpendicularOffset shifts centroid perpendicular to dir by offsetM metres.
// Perpendicular bearing = dir + 90° (right of travel); negative offsetM → left.
func ApplyPerpendicularOffset(centroid LatLng, dir Direction, offsetM float64) LatLng {
	perpDeg := float64(dir) + 90.0
	perpRad := perpDeg * math.Pi / 180
	dLat := offsetM * math.Cos(perpRad) / 111_000
	cosLat := math.Cos(centroid.Lat * math.Pi / 180)
	dLng := 0.0
	if cosLat != 0 {
		dLng = offsetM * math.Sin(perpRad) / (111_000 * cosLat)
	}
	return LatLng{Lat: centroid.Lat + dLat, Lng: centroid.Lng + dLng}
}

// ApplyPolygonOffsets converts centroid-relative OffsetM vertices to Leaflet [lat, lng] pairs.
// Returns [][2]float64 matching conditions_engine.Zone.Polygon type.
func ApplyPolygonOffsets(centroid LatLng, offsets []OffsetM) [][2]float64 {
	if len(offsets) == 0 {
		return [][2]float64{}
	}
	cosLat := math.Cos(centroid.Lat * math.Pi / 180)
	result := make([][2]float64, len(offsets))
	for i, o := range offsets {
		dLat := o.NorthM / 111_000
		dLng := 0.0
		if cosLat != 0 {
			dLng = o.EastM / (111_000 * cosLat)
		}
		result[i] = [2]float64{centroid.Lat + dLat, centroid.Lng + dLng}
	}
	return result
}

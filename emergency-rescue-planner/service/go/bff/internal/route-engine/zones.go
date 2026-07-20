package route_engine

// ZoneStore maps zone IDs to polygon vertices (lat/lng pairs, counter-clockwise).
type ZoneStore interface {
	GetPolygon(zoneID string) ([]LatLng, bool)
}

type MockZoneStore struct {
	polygons map[string][]LatLng
}

func NewMockZoneStore() *MockZoneStore {
	return &MockZoneStore{polygons: mockZones}
}

func (s *MockZoneStore) GetPolygon(zoneID string) ([]LatLng, bool) {
	p, ok := s.polygons[zoneID]
	return p, ok
}

var mockZones = map[string][]LatLng{
	// Melbourne CBD core (Swanston St / Bourke St intersection area)
	"zone-cbd-001": {
		{Lat: -37.8100, Lng: 144.9620},
		{Lat: -37.8100, Lng: 144.9720},
		{Lat: -37.8180, Lng: 144.9720},
		{Lat: -37.8180, Lng: 144.9620},
	},
	// Docklands / Western Melbourne
	"zone-docklands-001": {
		{Lat: -37.8150, Lng: 144.9320},
		{Lat: -37.8150, Lng: 144.9450},
		{Lat: -37.8250, Lng: 144.9450},
		{Lat: -37.8250, Lng: 144.9320},
	},
	// Southbank / South Melbourne
	"zone-southbank-001": {
		{Lat: -37.8240, Lng: 144.9580},
		{Lat: -37.8240, Lng: 144.9700},
		{Lat: -37.8320, Lng: 144.9700},
		{Lat: -37.8320, Lng: 144.9580},
	},
	// Richmond / inner east
	"zone-richmond-001": {
		{Lat: -37.8170, Lng: 144.9900},
		{Lat: -37.8170, Lng: 145.0000},
		{Lat: -37.8270, Lng: 145.0000},
		{Lat: -37.8270, Lng: 144.9900},
	},
}

// pointInPolygon reports whether (lat, lng) is inside polygon using ray casting.
// polygon is a slice of LatLng vertices (open ring — first ≠ last).
func PointInPolygon(lat, lng float64, polygon []LatLng) bool {
	crossings := 0
	n := len(polygon)
	for i := 0; i < n; i++ {
		a := polygon[i]
		b := polygon[(i+1)%n]
		if (a.Lat > lat) != (b.Lat > lat) {
			xIntersect := (b.Lng-a.Lng)*(lat-a.Lat)/(b.Lat-a.Lat) + a.Lng
			if lng < xIntersect {
				crossings++
			}
		}
	}
	return crossings%2 == 1
}

// computeAffectedZones returns the subset of zoneIDs whose polygons are
// intersected by any coordinate in lineCoords.
// lineCoords is in GeoJSON [lng, lat] order.
func computeAffectedZones(lineCoords [][]float64, zoneIDs []string, store ZoneStore) []string {
	affected := []string{}
	for _, zoneID := range zoneIDs {
		polygon, ok := store.GetPolygon(zoneID)
		if !ok {
			continue
		}
		for _, coord := range lineCoords {
			if len(coord) < 2 {
				continue
			}
			if PointInPolygon(coord[1], coord[0], polygon) {
				affected = append(affected, zoneID)
				break
			}
		}
	}
	return affected
}

package conditions_engine

import (
	"context"
	"math"
	"sync"
	"time"
)

// MockZoneRepository is an in-memory ZoneRepository implementation.
type MockZoneRepository struct {
	mu    sync.RWMutex
	zones map[string]*Zone
}

// NewMockZoneRepository creates an empty MockZoneRepository.
func NewMockZoneRepository() *MockZoneRepository {
	return &MockZoneRepository{zones: make(map[string]*Zone)}
}

func (r *MockZoneRepository) GetZonesNear(ctx context.Context, lat, lng, radiusKm float64) ([]Zone, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Zone, 0)
	for _, z := range r.zones {
		clat, clng := centroid(z.Polygon)
		if haversineKm(lat, lng, clat, clng) <= radiusKm {
			result = append(result, *z)
		}
	}
	return result, nil
}

func (r *MockZoneRepository) GetByID(ctx context.Context, id string) (Zone, error) {
	if err := ctx.Err(); err != nil {
		return Zone{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	z, ok := r.zones[id]
	if !ok {
		return Zone{}, ErrZoneNotFound
	}
	return *z, nil
}

func (r *MockZoneRepository) GetAll(ctx context.Context) ([]Zone, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Zone, 0, len(r.zones))
	for _, z := range r.zones {
		result = append(result, *z)
	}
	return result, nil
}

func (r *MockZoneRepository) UpsertZone(ctx context.Context, zone Zone) (Zone, error) {
	if err := ctx.Err(); err != nil {
		return Zone{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	zone.UpdatedAt = time.Now()
	poly := make([][2]float64, len(zone.Polygon))
	copy(poly, zone.Polygon)
	zone.Polygon = poly
	stored := zone
	r.zones[zone.ID] = &stored
	return stored, nil
}

func (r *MockZoneRepository) DeleteZone(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.zones[id]; !ok {
		return ErrZoneNotFound
	}
	delete(r.zones, id)
	return nil
}

func (r *MockZoneRepository) DeleteZonesBySource(ctx context.Context, source string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var deleted []string
	for id, z := range r.zones {
		if z.Source == source {
			delete(r.zones, id)
			deleted = append(deleted, id)
		}
	}
	if deleted == nil {
		deleted = make([]string, 0)
	}
	return deleted, nil
}

// centroid returns the arithmetic mean lat/lng of the polygon vertices.
func centroid(polygon [][2]float64) (lat, lng float64) {
	if len(polygon) == 0 {
		return 0, 0
	}
	for _, p := range polygon {
		lat += p[0]
		lng += p[1]
	}
	n := float64(len(polygon))
	return lat / n, lng / n
}

// haversineKm returns the great-circle distance in km between two lat/lng points.
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371.0
	dlat := (lat2 - lat1) * math.Pi / 180
	dlng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dlng/2)*math.Sin(dlng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

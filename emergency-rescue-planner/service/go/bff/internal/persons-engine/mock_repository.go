package persons_engine

import (
	"context"
	"math"
	"sync"
)

// MockPersonRepository is an in-memory PersonRepository implementation.
type MockPersonRepository struct {
	mu      sync.RWMutex
	persons map[string]*Person
}

// NewMockPersonRepository creates an empty MockPersonRepository.
func NewMockPersonRepository() *MockPersonRepository {
	return &MockPersonRepository{persons: make(map[string]*Person)}
}

// upsert inserts or replaces a person by ID. Used only by seedPersons — not
// part of the PersonRepository interface.
func (r *MockPersonRepository) upsert(p Person) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := p
	if stored.Needs == nil {
		stored.Needs = make([]string, 0)
	}
	r.persons[p.ID] = &stored
}

func (r *MockPersonRepository) GetPersonsNear(ctx context.Context, lat, lng, radiusKm float64) ([]Person, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Person, 0)
	for _, p := range r.persons {
		if haversineKm(lat, lng, p.Lat, p.Lng) <= radiusKm {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (r *MockPersonRepository) GetByID(ctx context.Context, id string) (Person, error) {
	if err := ctx.Err(); err != nil {
		return Person{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.persons[id]
	if !ok {
		return Person{}, ErrPersonNotFound
	}
	return *p, nil
}

func (r *MockPersonRepository) UpsertPerson(ctx context.Context, p Person) (Person, error) {
	if err := ctx.Err(); err != nil {
		return Person{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := p
	if stored.Needs == nil {
		stored.Needs = make([]string, 0)
	}
	r.persons[p.ID] = &stored
	return stored, nil
}

func (r *MockPersonRepository) DeletePerson(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.persons[id]; !ok {
		return ErrPersonNotFound
	}
	delete(r.persons, id)
	return nil
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

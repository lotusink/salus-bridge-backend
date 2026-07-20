package active_route_engine

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	route_engine "bff/internal/route-engine"
)

// MockActiveRouteRepository is an in-memory ActiveRouteRepository.
type MockActiveRouteRepository struct {
	mu                sync.RWMutex
	routes            map[string]*ActiveRoute     // id → route
	sessionIndex      map[string]string           // volunteerSession → id
	proposals         map[string]*RerouteProposal // proposalID → proposal
	acceptedProposals map[string]struct{}         // proposalID → consumed
}

func NewMockActiveRouteRepository() *MockActiveRouteRepository {
	return &MockActiveRouteRepository{
		routes:            make(map[string]*ActiveRoute),
		sessionIndex:      make(map[string]string),
		proposals:         make(map[string]*RerouteProposal),
		acceptedProposals: make(map[string]struct{}),
	}
}

func newActiveRouteID() string {
	return fmt.Sprintf("ar-%d-%d", time.Now().UnixNano(), rand.Int63n(10000))
}

func newProposalID() string {
	return fmt.Sprintf("prop-%d-%d", time.Now().UnixNano(), rand.Int63n(10000))
}

func (r *MockActiveRouteRepository) Register(ctx context.Context, route ActiveRoute) (ActiveRoute, error) {
	if err := ctx.Err(); err != nil {
		return ActiveRoute{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Delete existing route for this session
	if oldID, ok := r.sessionIndex[route.VolunteerSession]; ok {
		delete(r.routes, oldID)
	}
	route.ID = newActiveRouteID()
	route.RegisteredAt = time.Now()
	r.routes[route.ID] = &route
	r.sessionIndex[route.VolunteerSession] = route.ID
	return route, nil
}

func (r *MockActiveRouteRepository) GetByID(ctx context.Context, id string) (ActiveRoute, error) {
	if err := ctx.Err(); err != nil {
		return ActiveRoute{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if route, ok := r.routes[id]; ok {
		return *route, nil
	}
	return ActiveRoute{}, ErrActiveRouteNotFound
}

func (r *MockActiveRouteRepository) GetAll(ctx context.Context) ([]ActiveRoute, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ActiveRoute, 0, len(r.routes))
	for _, route := range r.routes {
		result = append(result, *route)
	}
	return result, nil
}

func (r *MockActiveRouteRepository) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	route, ok := r.routes[id]
	if !ok {
		return ErrActiveRouteNotFound
	}
	delete(r.sessionIndex, route.VolunteerSession)
	delete(r.routes, id)
	return nil
}

func (r *MockActiveRouteRepository) StoreProposal(ctx context.Context, proposal RerouteProposal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.proposals[proposal.ID] = &proposal
	return nil
}

func (r *MockActiveRouteRepository) GetProposal(ctx context.Context, proposalID string) (RerouteProposal, error) {
	if err := ctx.Err(); err != nil {
		return RerouteProposal{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.proposals[proposalID]
	if !ok {
		return RerouteProposal{}, ErrProposalNotFound
	}
	if time.Since(p.CreatedAt) > ProposalTTL {
		return RerouteProposal{}, ErrProposalExpired
	}
	return *p, nil
}

func (r *MockActiveRouteRepository) GetBySession(ctx context.Context, volunteerSession string) (ActiveRoute, error) {
	if err := ctx.Err(); err != nil {
		return ActiveRoute{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.sessionIndex[volunteerSession]
	if !ok {
		return ActiveRoute{}, ErrActiveRouteNotFound
	}
	route, ok := r.routes[id]
	if !ok {
		return ActiveRoute{}, ErrActiveRouteNotFound
	}
	return *route, nil
}

func (r *MockActiveRouteRepository) ApplyFreshRoute(ctx context.Context, activeRouteID, proposalID string, newRoute route_engine.RouteData, newWaypoints []LatLng) (ActiveRoute, error) {
	if err := ctx.Err(); err != nil {
		return ActiveRoute{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	route, ok := r.routes[activeRouteID]
	if !ok {
		return ActiveRoute{}, ErrActiveRouteNotFound
	}
	// Idempotent: already consumed → return current route unchanged.
	if _, consumed := r.acceptedProposals[proposalID]; consumed {
		return *route, nil
	}
	route.Geometry = newRoute.Geometry
	route.DurationSeconds = newRoute.DurationSeconds
	route.DistanceMetres = newRoute.DistanceMetres
	route.BBox = newRoute.BBox
	// Replace waypoints with the filtered remaining set; treat nil as empty.
	if newWaypoints == nil {
		route.Waypoints = []LatLng{}
	} else {
		route.Waypoints = append([]LatLng(nil), newWaypoints...) // defensive copy
	}
	r.acceptedProposals[proposalID] = struct{}{}
	return *route, nil
}

func (r *MockActiveRouteRepository) AcceptProposal(ctx context.Context, activeRouteID, proposalID string) (ActiveRoute, error) {
	if err := ctx.Err(); err != nil {
		return ActiveRoute{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Idempotent: already accepted
	if _, consumed := r.acceptedProposals[proposalID]; consumed {
		route, ok := r.routes[activeRouteID]
		if !ok {
			return ActiveRoute{}, ErrActiveRouteNotFound
		}
		return *route, nil
	}
	p, ok := r.proposals[proposalID]
	if !ok {
		return ActiveRoute{}, ErrProposalNotFound
	}
	if time.Since(p.CreatedAt) > ProposalTTL {
		return ActiveRoute{}, ErrProposalExpired
	}
	route, ok := r.routes[activeRouteID]
	if !ok {
		return ActiveRoute{}, ErrActiveRouteNotFound
	}
	// Replace geometry in-place; ID unchanged
	route.Geometry = p.NewRoute.Geometry
	route.DurationSeconds = p.NewRoute.DurationSeconds
	route.DistanceMetres = p.NewRoute.DistanceMetres
	route.BBox = p.NewRoute.BBox
	r.acceptedProposals[proposalID] = struct{}{}
	return *route, nil
}

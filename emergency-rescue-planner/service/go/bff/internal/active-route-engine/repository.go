package active_route_engine

import (
	"context"

	route_engine "bff/internal/route-engine"
)

// ActiveRouteRepository is the data access boundary for active volunteer routes.
// All implementations must be safe for concurrent use from multiple goroutines.
//
// Return semantics:
//   - GetAll never returns nil; returns empty slice when the store is empty.
//   - Infrastructure errors are non-nil error values.
//   - context.Canceled / context.DeadlineExceeded propagate without wrapping.
type ActiveRouteRepository interface {
	// Register stores route, replacing any existing route for the same
	// VolunteerSession. Returns the stored route (with server-generated ID).
	Register(ctx context.Context, route ActiveRoute) (ActiveRoute, error)

	// GetByID returns the active route identified by id.
	// Returns ErrActiveRouteNotFound if no route with that id exists.
	GetByID(ctx context.Context, id string) (ActiveRoute, error)

	// GetAll returns all active routes across all sessions. Never returns nil.
	GetAll(ctx context.Context) ([]ActiveRoute, error)

	// Delete removes the active route identified by id.
	// Returns ErrActiveRouteNotFound if no route with that id exists.
	Delete(ctx context.Context, id string) error

	// StoreProposal persists a reroute proposal under proposal.ID.
	// Overwrites any existing entry for the same ID.
	StoreProposal(ctx context.Context, proposal RerouteProposal) error

	// GetProposal returns the proposal identified by proposalID.
	// Returns ErrProposalNotFound if no such proposal.
	// Returns ErrProposalExpired if time.Since(proposal.CreatedAt) > ProposalTTL (5 min).
	GetProposal(ctx context.Context, proposalID string) (RerouteProposal, error)

	// AcceptProposal atomically replaces the active route identified by
	// activeRouteID with the geometry in proposalID. The route ID is unchanged.
	// Idempotent: a second call for the same proposalID returns the current
	// active route with nil error.
	// Returns ErrProposalNotFound or ErrProposalExpired if the proposal is
	// unavailable; ErrActiveRouteNotFound if the route was deleted.
	AcceptProposal(ctx context.Context, activeRouteID, proposalID string) (ActiveRoute, error)

	// GetBySession returns the active route registered for the given volunteerSession.
	// Returns ErrActiveRouteNotFound if no route is registered for that session.
	GetBySession(ctx context.Context, volunteerSession string) (ActiveRoute, error)

	// ApplyFreshRoute atomically marks proposalID as consumed and updates the
	// active route geometry with newRoute. newWaypoints replaces the route's
	// waypoint list — callers pass the filtered remaining waypoints so the
	// stored route reflects what's still ahead of the volunteer (already-
	// visited waypoints are dropped). Returns the updated active route.
	// Idempotent: if proposalID was already consumed, returns the current active
	// route without modifying geometry or waypoints.
	// Returns ErrActiveRouteNotFound if the active route was deleted.
	ApplyFreshRoute(ctx context.Context, activeRouteID, proposalID string, newRoute route_engine.RouteData, newWaypoints []LatLng) (ActiveRoute, error)
}

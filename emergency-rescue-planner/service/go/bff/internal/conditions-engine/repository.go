package conditions_engine

import (
	"context"
	"errors"
)

// ErrZoneNotFound is returned by GetByID and DeleteZone when no zone with
// the given id exists in the repository.
var ErrZoneNotFound = errors.New("zone not found")

// ZoneRepository is the data access boundary for risk zones. All
// implementations must be safe for concurrent use from multiple goroutines.
//
// Return semantics:
//   - An empty result set is returned as a non-nil, zero-length []Zone slice,
//     never nil. Callers may safely range over the result.
//   - Infrastructure errors are returned as non-nil error values.
//   - context.Canceled and context.DeadlineExceeded propagate from the
//     provided context without wrapping; inspect with errors.Is.
type ZoneRepository interface {
	// GetZonesNear returns all zones whose polygon centroids are within
	// radiusKm kilometres of (lat, lng). Never returns nil.
	GetZonesNear(ctx context.Context, lat, lng, radiusKm float64) ([]Zone, error)

	// GetByID returns the zone identified by id.
	// Returns ErrZoneNotFound if no zone with that id exists.
	GetByID(ctx context.Context, id string) (Zone, error)

	// GetAll returns all zones in the repository.
	// Never returns nil; returns an empty slice when the repository is empty.
	GetAll(ctx context.Context) ([]Zone, error)

	// UpsertZone creates or replaces the zone identified by zone.ID.
	// Returns the stored zone on success.
	UpsertZone(ctx context.Context, zone Zone) (Zone, error)

	// DeleteZone removes the zone identified by id.
	// Returns ErrZoneNotFound if no zone with that id exists.
	DeleteZone(ctx context.Context, id string) error

	// DeleteZonesBySource removes all zones whose Source field equals source.
	// Returns the IDs of all deleted zones (empty slice when none match).
	// Never returns ErrZoneNotFound.
	DeleteZonesBySource(ctx context.Context, source string) ([]string, error)
}

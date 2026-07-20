package persons_engine

import (
	"context"
	"errors"
)

// ErrPersonNotFound is returned by GetByID when no person with the given id
// exists in the repository.
var ErrPersonNotFound = errors.New("person not found")

// PersonRepository is the data access boundary for vulnerable persons. All
// implementations must be safe for concurrent use from multiple goroutines.
//
// Return semantics:
//   - An empty result set is returned as a non-nil, zero-length []Person slice,
//     never nil. Callers may safely range over the result.
//   - Infrastructure errors are returned as non-nil error values.
//   - context.Canceled and context.DeadlineExceeded propagate from the
//     provided context without wrapping; inspect with errors.Is.
type PersonRepository interface {
	// GetPersonsNear returns all persons within radiusKm kilometres of (lat, lng).
	// Never returns nil.
	GetPersonsNear(ctx context.Context, lat, lng, radiusKm float64) ([]Person, error)

	// GetByID returns the person identified by id.
	// Returns ErrPersonNotFound if no person with that id exists.
	GetByID(ctx context.Context, id string) (Person, error)

	// UpsertPerson creates or replaces the person identified by person.ID.
	// Returns the stored person on success.
	// Never returns nil Person on nil error.
	UpsertPerson(ctx context.Context, person Person) (Person, error)

	// DeletePerson removes the person identified by id.
	// Returns ErrPersonNotFound if no person with that id exists.
	DeletePerson(ctx context.Context, id string) error
}

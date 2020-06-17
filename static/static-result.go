package auth_static

import identity "github.com/aperturerobotics/identity"

type staticEntityLookupResult struct {
	err      error
	notFound bool
	entity   *identity.Entity
}

// newLookupEntityValue constructs a lookup entity static result.
func newLookupEntityValue(
	entity *identity.Entity,
	notFound bool,
	err error,
) *staticEntityLookupResult {
	return &staticEntityLookupResult{
		err:      err,
		notFound: notFound,
		entity:   entity,
	}
}

// GetError returns any overall error with the process.
func (r *staticEntityLookupResult) GetError() error {
	return r.err
}

// IsNotFound indicates if the result was not-found.
// If this is set and err != nil, err must be a not found error.
func (r *staticEntityLookupResult) IsNotFound() bool {
	return r.notFound
}

// GetEntity returns the entity record that was found.
func (r *staticEntityLookupResult) GetEntity() *identity.Entity {
	return r.entity
}

// _ is a type assertion
var _ identity.IdentityLookupEntityValue = ((*staticEntityLookupResult)(nil))

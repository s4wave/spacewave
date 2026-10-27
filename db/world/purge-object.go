package world

import (
	"context"

	"github.com/pkg/errors"
)

// ObjectPurger is a WorldState that can remove an object whose record or
// blocks the store lacks.
type ObjectPurger interface {
	// PurgeObject removes the object entry at key and every graph quad naming
	// it, without reading the object's record or root. The deletion change
	// keeps no reference to the record, so nothing still reaches its blocks.
	// Returns false, nil if not found.
	PurgeObject(ctx context.Context, key string) (bool, error)
}

// ErrPurgeUnsupported is returned when the WorldState cannot purge objects.
var ErrPurgeUnsupported = errors.New("world state cannot purge objects")

// PurgeObject purges the object at key through ws when it is an ObjectPurger.
func PurgeObject(ctx context.Context, ws WorldState, key string) (bool, error) {
	p, ok := ws.(ObjectPurger)
	if !ok {
		return false, ErrPurgeUnsupported
	}
	return p.PurgeObject(ctx, key)
}

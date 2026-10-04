package execution_controller

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
)

// laggingObjectState exposes an earlier root without changing the real object.
type laggingObjectState struct {
	world.ObjectState
	// rootRef is the earlier root exposed by this handle.
	rootRef *bucket.ObjectRef
	// rev identifies the earlier root revision.
	rev uint64
	// reads counts accesses that bypass the supplied watch snapshot.
	reads int
}

// GetRootRef reports the earlier snapshot and records its access.
func (s *laggingObjectState) GetRootRef(context.Context) (*bucket.ObjectRef, uint64, error) {
	s.reads++
	return s.rootRef, s.rev, nil
}

// _ is a type assertion.
var _ world.ObjectState = (*laggingObjectState)(nil)

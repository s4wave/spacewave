package space_world_replica

import "github.com/pkg/errors"

// Validate checks that the spec names a SharedObject and a desired owner.
func (s *ReplicaSpec) Validate() error {
	if s.GetSharedObjectId() == "" {
		return errors.New("replica spec names no shared object")
	}
	if s.GetDesiredOwnerPeerId() == "" {
		return errors.New("replica spec names no desired owner")
	}
	return nil
}

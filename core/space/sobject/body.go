package space_sobject

import (
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/world"
)

// spaceBody implements space.SpaceSharedObjectBody
type spaceBody struct {
	ref         *sobject.SharedObjectRef
	engineID    string
	bucketID    string
	volumeID    string
	sharedObj   sobject.SharedObject
	worldEngine world.Engine
}

// NewSpaceBody constructs a new space body implementation.
func NewSpaceBody(
	ref *sobject.SharedObjectRef,
	engineID string,
	bucketID string,
	volumeID string,
	sharedObj sobject.SharedObject,
	worldEngine world.Engine,
) space.SpaceSharedObjectBody {
	return &spaceBody{
		ref:         ref,
		engineID:    engineID,
		bucketID:    bucketID,
		volumeID:    volumeID,
		sharedObj:   sharedObj,
		worldEngine: worldEngine,
	}
}

// GetWorldEngine returns the world engine for this space.
func (s *spaceBody) GetWorldEngine() world.Engine {
	return s.worldEngine
}

// GetWorldEngineID returns the world engine identifier for this space.
func (s *spaceBody) GetWorldEngineID() string {
	return s.engineID
}

// GetWorldEngineBucketID returns the bucket ID for the world engine.
func (s *spaceBody) GetWorldEngineBucketID() string {
	return s.bucketID
}

// GetWorldEngineVolumeID returns the volume ID for the world engine.
func (s *spaceBody) GetWorldEngineVolumeID() string {
	return s.volumeID
}

// GetSharedObjectRef returns the shared object reference for this space.
func (s *spaceBody) GetSharedObjectRef() *sobject.SharedObjectRef {
	return s.ref
}

// GetSharedObject returns the shared object handle.
func (s *spaceBody) GetSharedObject() sobject.SharedObject {
	return s.sharedObj
}

// _ is a type assertion
var _ space.SpaceSharedObjectBody = ((*spaceBody)(nil))

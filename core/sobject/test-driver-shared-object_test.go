package sobject

import (
	"github.com/s4wave/spacewave/net/peer"
)

// testDriverSharedObject is a SharedObject that only names its local peer,
// for a BodyDriver stepped directly.
type testDriverSharedObject struct {
	// SharedObject is nil: any other method panics.
	SharedObject
	// peerID is the local peer.
	peerID peer.ID
}

// GetPeerID returns the local peer.
func (s *testDriverSharedObject) GetPeerID() peer.ID {
	return s.peerID
}

// GetSharedObjectID returns the mock shared object ID.
func (s *testDriverSharedObject) GetSharedObjectID() string {
	return mockSharedObjectID
}

// _ is a type assertion
var _ SharedObject = (*testDriverSharedObject)(nil)

//go:build !js

package bldr_project_controller

import (
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/db/world"
)

// RemoteRef is a reference to a remote.
type RemoteRef struct {
	ref     *keyed.KeyedRef[string, *remoteTracker]
	tracker *remoteTracker
}

// newRemoteRef constructs a RemoteRef.
func newRemoteRef(ref *keyed.KeyedRef[string, *remoteTracker], tracker *remoteTracker) *RemoteRef {
	return &RemoteRef{ref: ref, tracker: tracker}
}

// GetRemoteID returns the ID of the remote.
func (r *RemoteRef) GetRemoteID() string {
	return r.tracker.remoteID
}

// GetRemoteConfig returns the config of the remote.
func (r *RemoteRef) GetRemoteConfig() *bldr_project.RemoteConfig {
	return r.tracker.remote
}

// GetResultPromise returns the result promise.
func (r *RemoteRef) GetResultPromise() promise.PromiseLike[*world.Engine] {
	return r.tracker.resultPromise
}

// Release releases the reference.
func (r *RemoteRef) Release() {
	r.ref.Release()
}

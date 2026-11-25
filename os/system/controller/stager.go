package os_system_controller

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	os_release "github.com/s4wave/spacewave/os/release"
	os_system_stager "github.com/s4wave/spacewave/os/system/stager"
)

// stager serves the release the node binds over one World engine.
type stager struct {
	// ctx ends when the engine stops serving; every stream ends with it.
	ctx context.Context
	// eng is the World engine holding the release.
	eng world.Engine
	// key is the release's object key.
	key string
}

// WatchRelease sends the release, and again each time it changes. A release
// that cannot be read ends the stream with its error, since the console can
// stage nothing from it; the console retries after its backoff.
func (s *stager) WatchRelease(_ *os_system_stager.WatchReleaseRequest, strm os_system_stager.SRPCStager_WatchReleaseStream) error {
	// End the stream with the engine as well as with the client.
	ctx, cancel := context.WithCancel(strm.Context())
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()

	// Send each readable release; wait while none exists.
	return watchRelease(ctx, s.eng, s.key, func(r *os_release.Release, err error) error {
		if errors.Is(err, world.ErrObjectNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return strm.Send(&os_system_stager.WatchReleaseResponse{Release: r})
	})
}

// watchRelease calls cb with the release at key, and again after each change
// to the object, until ctx ends or cb returns an error. A missing object
// calls cb with world.ErrObjectNotFound and waits for it to appear. Deleting
// the object calls nothing, so the Device keeps the image it runs.
func watchRelease(ctx context.Context, eng world.Engine, key string, cb func(*os_release.Release, error) error) error {
	ws := world.NewEngineWorldState(eng, false)
	for {
		// Read the release at its current revision.
		next, _, err := world.GetObjectRev(ctx, ws, key)
		if err != nil {
			return err
		}
		r, err := os_release.ReadRelease(ctx, ws, key)
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		if err := cb(r, err); err != nil {
			return err
		}

		// Wait for the next revision. A change between the two reads above
		// ends this wait at once and sends the same release again, which the
		// console skips.
		if _, err := world.WaitObjectRev(ctx, ws, key, next+1, true); err != nil {
			return err
		}
	}
}

// _ is a type assertion
var _ os_system_stager.SRPCStagerServer = (*stager)(nil)

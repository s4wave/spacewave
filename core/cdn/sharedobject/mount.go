package cdn_sharedobject

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// MountedWorld is a read-only World over the published head of a CDN Space,
// with the CDN transport it reads through.
type MountedWorld struct {
	world.WorldState
	// Store is the World's CDN transport. Its stats report the World's
	// downloads.
	Store *cdn_bstore.CdnBlockStore

	release func()
}

// Release closes the World and its transport.
func (m *MountedWorld) Release() {
	m.release()
}

// MountWorld opens a read-only World over the published head of the CDN Space
// spaceID served from baseURL. The World owns its CDN transport and reads
// blocks lazily as they are first used. It keeps the head it mounted, so
// objects stay readable while the CDN publishes a newer one.
func MountWorld(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	baseURL, spaceID string,
) (*MountedWorld, error) {
	// Own the CDN transport until the mounted World is released.
	store, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: baseURL,
		SpaceID:    spaceID,
		PointerTTL: -1,
	})
	if err != nil {
		return nil, errors.Wrap(err, "build cdn block store")
	}

	// Bind the published head to the transport's block store.
	so, err := NewCdnSharedObject(CdnSharedObjectOptions{
		SpaceID:    spaceID,
		Bus:        b,
		Logger:     le,
		BlockStore: store,
	})
	if err != nil {
		store.Close()
		return nil, errors.Wrap(err, "build cdn shared object")
	}

	// Keep the read engine and its transport under one release function.
	we, err := NewWorldEngine(ctx, le, b, so)
	if err != nil {
		store.Close()
		return nil, errors.Wrap(err, "mount cdn world")
	}
	return &MountedWorld{
		WorldState: world.NewEngineWorldState(we.Engine, false),
		Store:      store,
		release: func() {
			we.Release()
			store.Close()
		},
	}, nil
}

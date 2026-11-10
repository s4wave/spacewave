//go:build !js && !wasip1

package main

import (
	"context"

	"github.com/pkg/errors"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/testbed"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// benchRootName is the World root on the bench's Volume.
const benchRootName = "world:regen-bench"

// benchWorld is the bench's World: an engine on a local authority over an
// s4db Volume with the production collector.
type benchWorld struct {
	tb        *testbed.Testbed
	base      *bucket_lookup.Cursor
	root      *bucket_lookup.Cursor
	authority *world_block.LocalAuthority
	engine    *world_block.Engine
}

// openBenchWorld opens the World in the s4db file at path, creating both when
// absent. The testbed's bucket ID is fixed, so a reopen finds the same root.
func openBenchWorld(ctx context.Context, path string) (*benchWorld, error) {
	// Open the Volume quietly and the World's bucket on it.
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	le := logrus.NewEntry(log)
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVolumeConfig(&volume_s4db.Config{Path: path}))
	if err != nil {
		return nil, errors.Wrap(err, "open volume")
	}
	w := &benchWorld{tb: tb}
	if w.base, err = tb.BuildEmptyCursor(ctx); err != nil {
		w.Close()
		return nil, err
	}

	// Open the engine at the authority's head.
	scope := coord.Scope{VolumeID: tb.Volume.GetID(), ObjectStoreID: benchRootName, ParticipantID: "bench"}
	w.authority = world_block.NewLocalAuthority(le, w.base, benchRootName, tb.Volume, scope)
	head, err := w.authority.Head(ctx)
	if err != nil {
		w.Close()
		return nil, err
	}
	w.root = w.base.Clone()
	w.root.SetRootRef(head.Root)
	if w.engine, err = world_block.NewEngine(ctx, le, w.root, world_mock.LookupMockOp, w.authority, false); err != nil {
		w.Close()
		return nil, err
	}
	return w, nil
}

// Close closes the engine and the Volume.
func (w *benchWorld) Close() {
	if w.engine != nil {
		_ = w.engine.Close()
	}
	for _, c := range []*bucket_lookup.Cursor{w.root, w.base} {
		if c != nil {
			c.Release()
		}
	}
	w.tb.Release()
}

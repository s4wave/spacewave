package sobject_world_engine

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/blocktype"
	blocktype_controller "github.com/s4wave/spacewave/db/blocktype/controller"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// TestRetainPublicationWorldRecoversPeerDependencies exercises the engine's
// retention boundary with a peer-only nested payload and a separate local cache.
func TestRetainPublicationWorldRecoversPeerDependencies(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	remote, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(remote.Release)
	local, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(local.Release)
	source, err := remote.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(source.Release)
	target, err := local.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(target.Release)
	ws, err := world_block.BuildMockWorldState(ctx, le, true, source, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ws.Discard)
	leaf, _, err := block.PutBlock(ctx, source.GetBucket(), &block_mock.Example{Msg: "peer-only payload"})
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := block.PutBlock(ctx, source.GetBucket(), &block_mock.Root{ExampleSubBlock: &block_mock.SubBlock{ExamplePtr: leaf}})
	if err != nil {
		t.Fatal(err)
	}
	{
		createdObject, err := ws.CreateObject(ctx, "content", &bucket.ObjectRef{RootRef: root})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := world_types.SetObjectType(ctx, ws, "content", "test/nested"); err != nil {
		t.Fatal(err)
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	release, err := local.Bus.AddController(ctx, blocktype_controller.NewController(func(context.Context, string) (blocktype.BlockType, error) {
		return blocktype.NewBlockType("test/nested", block_mock.NewRootBlock), nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// The same overlay used by cloud stores exposes remote bytes on demand.
	overlay := block.NewOverlay(ctx, le, source.GetBucket(), target.GetBucket(), block.OverlayMode_UPPER_WRITE_CACHE, 0, nil)
	shared := &publicationTestSharedObject{
		blockStore: newTestBlockStore("publication-test", overlay),
		bus:        local.Bus, retained: newTestRejectedCandidateStore(),
	}
	c := &Controller{le: le, bus: local.Bus, sfs: local.StepFactorySet}
	head := &bucket.ObjectRef{RootRef: ws.GetRootRef()}
	if err := c.retainPublicationWorld(ctx, shared, head); err != nil {
		t.Fatal(err)
	}
	if _, found, err := target.GetBucket().GetBlock(ctx, leaf); err != nil || !found {
		t.Fatalf("peer dependency was not retained: found=%v, err=%v", found, err)
	}

	// Remove the peer path. The retained graph remains usable and a repeat
	// checkpoint resolves entirely from local storage and completion records.
	shared.blockStore = newTestBlockStore("publication-test", target.GetBucket())
	if err := c.retainPublicationWorld(ctx, shared, head); err != nil {
		t.Fatalf("peer loss invalidated the retained checkpoint: %v", err)
	}
}

type publicationTestSharedObject struct {
	testSharedObject
	bus      bus.Bus
	retained kvtx.Store
}

func (s *publicationTestSharedObject) GetBus() bus.Bus { return s.bus }

func (s *publicationTestSharedObject) AccessPublicationRetention(context.Context) (kvtx.Store, func(), error) {
	return s.retained, func() {}, nil
}

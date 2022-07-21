package unixfs_world

import (
	"context"
	"time"

	hydra_testbed "github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/hydra/world"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BuildTestbed builds a unixfs world testbed.
func BuildTestbed(ctx context.Context, watchWorldChanges bool, opts ...world_testbed.Option) (*unixfs.FS, *world_testbed.Testbed, error) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		return nil, nil, err
	}

	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		return nil, nil, err
	}

	ufs, err := InitTestbed(wtb, watchWorldChanges)
	if err != nil {
		return nil, wtb, err
	}

	return ufs, wtb, nil
}

// InitTestbed inits the testbed with a new fs.
func InitTestbed(tb *world_testbed.Testbed, watchWorldChanges bool) (*unixfs.FS, error) {
	ctx := tb.Context
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		return nil, err
	}

	// provide op handlers to bus
	engineID := tb.EngineID
	opc := world.NewLookupOpController("test-fs-ops", engineID, LookupFsOp)
	go func() {
		_ = tb.Bus.ExecuteController(ctx, opc)
	}()

	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	// uses directive to look up the engine
	eng := tb.Engine
	// uses short-lived engine txs to implement world state
	ws := world.NewEngineWorldState(ctx, eng, true)

	sender := tb.Volume.GetPeerID()
	objKey := "test-git-repo"
	fsType := FSType_FSType_FS_NODE
	err = FsInit(
		ctx,
		ws,
		sender,
		objKey,
		fsType,
		nil,
		0,
		true,
		time.Now(),
	)
	if err != nil {
		return nil, err
	}

	// check type
	ts := world_types.NewTypesState(ctx, ws)
	typeID, err := ts.GetObjectType(objKey)
	if err != nil {
		return nil, err
	}
	if typeID != FSNodeTypeID {
		return nil, errors.Errorf("expected type id %s but got %q", FSObjectTypeID, typeID)
	}

	// construct full fs
	tb.Logger.Debugf("filesystem initialized w/ type: %s", typeID)
	writer := NewFSWriter(ws, objKey, fsType, sender)
	rootFSCursor := NewFSCursor(tb.Logger, ws, objKey, fsType, writer, watchWorldChanges)
	ufs := unixfs.NewFS(ctx, tb.Logger, rootFSCursor, nil)
	return ufs, nil
}

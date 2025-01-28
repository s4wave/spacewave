package unixfs_world_testbed

import (
	"time"

	hydra_testbed "github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	"github.com/aperturerobotics/hydra/world"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	world_types "github.com/aperturerobotics/hydra/world/types"
)

// BuildTestbed builds a unixfs world testbed.
func BuildTestbed(
	tb *hydra_testbed.Testbed,
	objKey string,
	watchWorldChanges bool,
	opts ...world_testbed.Option,
) (*unixfs.FSHandle, *world_testbed.Testbed, error) {
	wtb, err := world_testbed.NewTestbed(tb, opts...)
	if err != nil {
		return nil, nil, err
	}

	ufs, err := InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		return nil, wtb, err
	}

	return ufs, wtb, nil
}

// InitTestbed inits the testbed with a new fs.
//
// if watchWorldChanges is nil the fs will be read-only
func InitTestbed(
	tb *world_testbed.Testbed,
	objKey string,
	watchWorldChanges bool,
) (*unixfs.FSHandle, error) {
	ctx := tb.Context

	// provide op handlers to bus
	engineID := tb.EngineID
	opc := world.NewLookupOpController("test-fs-ops", engineID, unixfs_world.LookupFsOp)
	go func() {
		_ = tb.Bus.ExecuteController(ctx, opc)
	}()

	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	// uses directive to look up the engine
	eng := tb.Engine
	// uses short-lived engine txs to implement world state
	ws := world.NewEngineWorldState(eng, true)

	sender := tb.Volume.GetPeerID()
	fsType := unixfs_world.FSType_FSType_FS_NODE
	typeID, _ := unixfs_world.FSTypeToTypeID(fsType)
	_, _, err := unixfs_world.FsInit(
		ctx,
		ws,
		sender,
		objKey,
		fsType,
		nil,
		true,
		time.Now(),
	)
	if err != nil {
		return nil, err
	}

	// check type
	if err := world_types.CheckObjectType(ctx, ws, objKey, typeID); err != nil {
		return nil, err
	}

	// construct full fs
	tb.Logger.Debug("filesystem initialized")
	rootFSCursor, err := unixfs_world.FollowUnixfsRef(ctx, tb.Logger, ws, &unixfs_world.UnixfsRef{ObjectKey: objKey}, sender, watchWorldChanges)
	if err != nil {
		return nil, err
	}
	return unixfs.NewFSHandle(rootFSCursor)
}

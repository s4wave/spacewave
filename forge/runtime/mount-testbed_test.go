package forge_runtime

import (
	"context"
	"testing"
	"time"

	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_sdk "github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// workdirTestbed releases the hydra and world testbeds backing one test FSHandle.
type workdirTestbed struct {
	btb *hydra_testbed.Testbed
	wtb *world_testbed.Testbed
}

// Release tears down the testbeds.
func (w *workdirTestbed) Release() {
	w.wtb.Release()
	w.btb.Release()
}

// buildTestWorkdirHandle constructs a live empty Workdir FSHandle over a real
// world engine, mirroring the production init path.
func buildTestWorkdirHandle(ctx context.Context, t *testing.T) (*unixfs_sdk.FSHandle, *workdirTestbed, error) {
	t.Helper()
	log := logrus.New()
	le := logrus.NewEntry(log)
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		return nil, nil, err
	}
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		btb.Release()
		return nil, nil, err
	}
	out := &workdirTestbed{btb: btb, wtb: wtb}

	opc := world.NewLookupOpController(
		"test-workdir-fs-ops",
		wtb.EngineID,
		unixfs_world.LookupFsOp,
	)
	if _, err := wtb.Bus.AddController(ctx, opc, nil); err != nil {
		out.Release()
		return nil, nil, err
	}
	<-time.After(time.Millisecond * 100)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, _, err := unixfs_world.FsInit(
		ctx,
		ws,
		sender,
		"test-workdir-fs",
		unixfs_world.FSType_FSType_FS_NODE,
		nil,
		true,
		time.Now(),
	); err != nil {
		out.Release()
		return nil, nil, err
	}
	rootCursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		wtb.Logger,
		ws,
		&unixfs_world.UnixfsRef{ObjectKey: "test-workdir-fs"},
		sender,
		true,
	)
	if err != nil {
		out.Release()
		return nil, nil, err
	}
	handle, err := unixfs_sdk.NewFSHandle(rootCursor)
	if err != nil {
		out.Release()
		return nil, nil, err
	}
	return handle, out, nil
}

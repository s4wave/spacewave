package space_world_ops_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/world"
	world_opcheck "github.com/s4wave/spacewave/db/world/opcheck"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
)

// TestSpaceOpsConform checks the built-in Space operations apply, rehearse and
// replay alike and stay off the network.
func TestSpaceOpsConform(t *testing.T) {
	ts := time.Unix(1_800_000_000, 0)
	sender := peer.ID("opcheck-sender")
	initCanvas := func(ctx context.Context, ws world.WorldState) error {
		_, _, err := ws.ApplyWorldOp(ctx, space_world_ops.NewCanvasInitOp("canvas/a", ts), sender)
		return err
	}
	node := &s4wave_canvas.CanvasNode{Id: "n1", X: 1, Y: 2, Width: 30, Height: 40}
	cases := map[string]world_opcheck.Case{
		"set-space-settings": {Op: space_world_ops.NewSetSpaceSettingsOp("", &space_world.SpaceSettings{IndexPath: "files"}, true, ts)},
		"set-index-path":     {Op: &space_world_ops.SetSpaceIndexPathOp{IndexPath: "files", Timestamp: timestamppb.New(ts)}},
		"init-unixfs":        {Op: space_world_ops.NewInitUnixFSOp("files", ts)},
		"init-object-layout": {Op: space_world_ops.NewInitObjectLayoutOp("layout/a", ts)},
		"init-canvas":        {Op: space_world_ops.NewCanvasInitOp("canvas/a", ts)},
		"init-canvas-demo":   {Op: space_world_ops.NewInitCanvasDemoOp("canvas/demo", ts)},
		"canvas-add-node":    {Setup: initCanvas, Op: space_world_ops.NewCanvasAddNodeOp("canvas/a", node)},
		"canvas-add-missing": {Op: space_world_ops.NewCanvasAddNodeOp("canvas/missing", node)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			c.Sender, c.LookupOp = sender, space_world_optypes.LookupWorldOp
			world_opcheck.Check(t, c)
		})
	}
}

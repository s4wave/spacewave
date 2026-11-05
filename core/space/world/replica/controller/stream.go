package space_world_replica_controller

import (
	"context"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	space_world_replica "github.com/s4wave/spacewave/core/space/world/replica"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/protocol"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
)

// watchRetryDelay is how long the node waits before watching the other node
// again after the stream ends.
const watchRetryDelay = time.Second

// serve runs the SOReplica service on the pair's protocol and returns its
// release. The server stays attached after ctx ends until it is released,
// since its Execute returns at once. Each watch it serves ends with ctx.
func (c *Controller) serve(ctx context.Context) (func(), error) {
	srv := &peerServer{c: c, ctx: ctx}
	server, err := stream_srpc_server.NewServer(
		c.b, c.le, c.GetControllerInfo(),
		[]stream_srpc_server.RegisterFn{func(mux srpc.Mux) error { return space_world_replica.SRPCRegisterSOReplica(mux, srv) }},
		[]protocol.ID{protocol.ID(c.conf.GetProtocolId())}, []string{c.local.String()}, false,
	)
	if err != nil {
		return nil, err
	}
	return c.b.AddController(ctx, server, nil)
}

// peerServer serves this node's state for one run of the controller.
type peerServer struct {
	// c is the controller.
	c *Controller
	// ctx is the run's context. A watch outliving the run would serve a
	// stopped node's spec to the other node.
	ctx context.Context
}

// WatchReplica streams this node's state to the other node, once at start and
// again on each change, until the stream or the run ends.
func (s *peerServer) WatchReplica(_ *space_world_replica.WatchReplicaRequest, strm space_world_replica.SRPCSOReplica_WatchReplicaStream) error {
	// Serve only the other node's Device.
	c, ctx := s.c, strm.Context()
	mounted, err := link.MustGetMountedStreamContext(ctx)
	if err != nil {
		return err
	}
	if mounted.GetPeerID() != c.remote {
		return errors.New("replica state watched by another peer")
	}

	// Send the state, then each change. The broadcast also wakes on the
	// other node's state, which leaves this node's unchanged, and resending it
	// would echo between the two nodes.
	var sent *space_world_replica.ReplicaState
	for {
		var st *space_world_replica.ReplicaState
		var waitCh <-chan struct{}
		c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			st, waitCh = c.state, getWaitCh()
		})
		if st != sent {
			if err := strm.Send(st); err != nil {
				return err
			}
			sent = st
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-s.ctx.Done():
			return context.Cause(s.ctx)
		case <-waitCh:
		}
	}
}

// watchPeer follows the other node's state until ctx ends, holding nil while
// the stream is down so no step acts on a stale state.
func (c *Controller) watchPeer(ctx context.Context) {
	for {
		// Follow the stream until it ends.
		err := c.followPeer(ctx)
		c.setPeerState(nil)
		if ctx.Err() != nil {
			return
		}
		c.le.WithError(err).Debug("replica peer stream ended")

		// Wait before watching again.
		select {
		case <-ctx.Done():
			return
		case <-time.After(watchRetryDelay):
		}
	}
}

// followPeer opens the other node's watch and records each state it sends.
func (c *Controller) followPeer(ctx context.Context) error {
	// Open the watch on the pair's protocol.
	open := stream_srpc.NewOpenStreamFunc(c.b, protocol.ID(c.conf.GetProtocolId()), c.local, c.remote, 0)
	strm, err := space_world_replica.NewSRPCSOReplicaClient(srpc.NewClient(open)).WatchReplica(ctx, &space_world_replica.WatchReplicaRequest{})
	if err != nil {
		return err
	}
	defer strm.Close()

	// Record each state.
	for {
		st, err := strm.Recv()
		if err != nil {
			return err
		}
		c.setPeerState(st)
	}
}

// setPeerState records the other node's state and wakes the reconcile loop.
func (c *Controller) setPeerState(st *space_world_replica.ReplicaState) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.peerState = st
		broadcast()
	})
}

// _ is a type assertion
var _ space_world_replica.SRPCSOReplicaServer = (*peerServer)(nil)

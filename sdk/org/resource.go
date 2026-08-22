package s4wave_org

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// OrgResource implements the OrgResourceService SRPC interface.
type OrgResource struct {
	ws     world.WorldState
	objKey string
	bcast  broadcast.Broadcast
	mux    srpc.Mux

	watch    *routine.RoutineContainer
	closed   bool
	state    *OrgState
	watchErr error
}

// NewOrgResource creates a new OrgResource.
// If ws and objKey are set, the resource watches the World object and
// streams state revisions to WatchOrgState callers.
func NewOrgResource(ws world.WorldState, objKey string, state *OrgState) *OrgResource {
	if state == nil {
		state = &OrgState{}
	}
	r := &OrgResource{
		ws:     ws,
		objKey: objKey,
		state:  state,
	}
	if ws != nil && objKey != "" {
		r.watch = routine.NewRoutineContainer()
		r.watch.SetRoutine(r.watchOrgWorld)
		r.watch.SetContext(context.Background(), false)
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterOrgResourceService(mux, r)
	})
	return r
}

// Close releases the org resource lifecycle.
func (r *OrgResource) Close() {
	if r.watch != nil {
		r.watch.ClearContext()
	}
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.closed = true
		r.watchErr = context.Canceled
		broadcast()
	})
}

// GetMux returns the srpc mux for this resource.
func (r *OrgResource) GetMux() srpc.Mux {
	return r.mux
}

// WatchOrgState streams organization state changes.
func (r *OrgResource) WatchOrgState(_ *WatchOrgStateRequest, strm SRPCOrgResourceService_WatchOrgStateStream) error {
	return broadcast.WatchBroadcastWithEqual(
		strm.Context(),
		&r.bcast,
		r.snapshotWatchStateLocked,
		func(snap *orgWatchSnapshot) error {
			if snap.err != nil {
				return snap.err
			}
			return strm.Send(&WatchOrgStateResponse{State: snap.state.CloneVT()})
		},
		orgWatchSnapshotsEqual,
	)
}

// orgWatchSnapshot is one watch emission: the state or the watch failure.
type orgWatchSnapshot struct {
	state *OrgState
	err   error
}

// orgWatchSnapshotsEqual reports whether two snapshots carry the same state.
func orgWatchSnapshotsEqual(a, b *orgWatchSnapshot) bool {
	if (a.err == nil) != (b.err == nil) {
		return false
	}
	if a.err != nil {
		return true
	}
	return a.state.EqualVT(b.state)
}

// watchOrgWorld re-reads the org state after each World sequence change.
func (r *OrgResource) watchOrgWorld(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			r.setWatchError(err)
			return err
		}

		seqno, err := r.ws.GetSeqno(ctx)
		if err != nil {
			r.setWatchError(err)
			return err
		}

		objState, found, err := r.ws.GetObject(ctx, r.objKey)
		if err != nil {
			r.setWatchError(err)
			return err
		}
		if !found {
			r.setWatchError(world.ErrObjectNotFound)
			return world.ErrObjectNotFound
		}
		state, err := func() (*OrgState, error) {
			// Release the object-state handle before the WaitSeqno block below
			// so the read scope does not span the wait.
			defer world.ReleaseObjectState(objState)
			var state *OrgState
			_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
				var uerr error
				state, uerr = UnmarshalOrgState(ctx, bcs)
				return uerr
			})
			if err != nil {
				return nil, err
			}
			if state == nil {
				state = &OrgState{}
			}
			return state, nil
		}()
		if err != nil {
			r.setWatchError(err)
			return err
		}
		r.setWatchState(state)

		_, err = r.ws.WaitSeqno(ctx, seqno+1)
		if err != nil {
			r.setWatchError(err)
			return err
		}
	}
}

// snapshotWatchStateLocked returns the current state or watch error.
func (r *OrgResource) snapshotWatchStateLocked() *orgWatchSnapshot {
	if r.watchErr != nil {
		return &orgWatchSnapshot{err: r.watchErr}
	}
	return &orgWatchSnapshot{state: r.state.CloneVT()}
}

// setWatchState publishes a re-read of the World object state.
func (r *OrgResource) setWatchState(state *OrgState) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if r.closed {
			return
		}
		r.state = state.CloneVT()
		r.watchErr = nil
		broadcast()
	})
}

// setWatchError records a watch failure and wakes watchers.
func (r *OrgResource) setWatchError(err error) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if r.closed {
			return
		}
		r.watchErr = err
		broadcast()
	})
}

var _ SRPCOrgResourceServiceServer = (*OrgResource)(nil)

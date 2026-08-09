package s4wave_wizard

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	space_uri "github.com/s4wave/spacewave/sdk/space"
)

// WizardResource implements the WizardResourceService SRPC interface.
type WizardResource struct {
	ws            world.WorldState
	engine        world.Engine
	objKey        string
	ctxCancel     context.CancelFunc
	cloneRoutine  *routine.RoutineContainer
	stateWatch    *routine.RoutineContainer
	state         *WizardState
	stateRev      uint64
	stateWatchErr error
	stateClosed   bool
	cloneProgress *GitCloneProgress
	bcast         broadcast.Broadcast
	mux           srpc.Mux
}

var wizardStateWatchBackoff = &backoff.Backoff{
	BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
	Exponential: &backoff.Exponential{
		InitialInterval: 100,
		MaxInterval:     1000,
	},
}

type wizardStateWatchSnapshot struct {
	state *WizardState
	err   error
}

type wizardReleasableObjectState interface {
	Release()
}

// NewWizardResource creates a new WizardResource.
func NewWizardResource(ws world.WorldState, engine world.Engine, objKey string, state *WizardState) *WizardResource {
	if state == nil {
		state = &WizardState{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cloneRoutine := routine.NewRoutineContainer()
	cloneRoutine.SetContext(ctx, false)
	r := &WizardResource{
		ws:           ws,
		engine:       engine,
		objKey:       objKey,
		ctxCancel:    cancel,
		cloneRoutine: cloneRoutine,
		state:        state,
		cloneProgress: &GitCloneProgress{
			State: GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_IDLE,
		},
	}
	if ws != nil && objKey != "" {
		r.stateWatch = routine.NewRoutineContainer(routine.WithRetry(wizardStateWatchBackoff))
		r.stateWatch.SetRoutine(r.watchWizardWorld)
		r.stateWatch.SetContext(ctx, false)
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterWizardResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *WizardResource) GetMux() srpc.Mux {
	return r.mux
}

// Close releases the wizard resource lifecycle.
func (r *WizardResource) Close() {
	r.cloneRoutine.ClearContext()
	if r.stateWatch != nil {
		r.stateWatch.ClearContext()
	}
	r.ctxCancel()
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.stateClosed = true
		r.stateWatchErr = context.Canceled
		broadcast()
	})
}

// WatchWizardState streams wizard state changes.
func (r *WizardResource) WatchWizardState(_ *WatchWizardStateRequest, strm SRPCWizardResourceService_WatchWizardStateStream) error {
	return broadcast.WatchBroadcastWithEqual(
		strm.Context(),
		&r.bcast,
		r.snapshotWizardStateWatchLocked,
		func(snap *wizardStateWatchSnapshot) error {
			if snap.err != nil {
				return snap.err
			}
			return strm.Send(&WatchWizardStateResponse{State: snap.state.CloneVT()})
		},
		wizardStateWatchSnapshotsEqual,
	)
}

func (r *WizardResource) watchWizardWorld(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			r.setWizardStateWatchError(err)
			return err
		}

		seqno, err := r.ws.GetSeqno(ctx)
		if err != nil {
			r.setWizardStateWatchError(err)
			return err
		}

		objState, found, err := r.ws.GetObject(ctx, r.objKey)
		if err != nil {
			r.setWizardStateWatchError(err)
			return err
		}
		if !found {
			r.setWizardStateWatchError(world.ErrObjectNotFound)
			return world.ErrObjectNotFound
		}

		state, rev, err := func() (*WizardState, uint64, error) {
			if rel, ok := objState.(wizardReleasableObjectState); ok {
				defer rel.Release()
			}
			_, rev, err := objState.GetRootRef(ctx)
			if err != nil {
				return nil, 0, err
			}
			state, err := r.readWizardWorldState(ctx, objState)
			return state, rev, err
		}()
		if err != nil {
			r.setWizardStateWatchError(err)
			return err
		}
		r.setWizardStateWatchState(state, rev)

		_, err = r.ws.WaitSeqno(ctx, seqno+1)
		if err != nil {
			r.setWizardStateWatchError(err)
			return err
		}
	}
}

func (r *WizardResource) readWizardWorldState(ctx context.Context, objState world.ObjectState) (*WizardState, error) {
	var state *WizardState
	_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = UnmarshalWizardState(ctx, bcs)
		return uerr
	})
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &WizardState{}
	}
	return state, nil
}

func (r *WizardResource) setWizardStateWatchState(state *WizardState, rev uint64) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if r.stateClosed || rev < r.stateRev {
			return
		}
		r.state = state.CloneVT()
		r.stateRev = rev
		r.stateWatchErr = nil
		broadcast()
	})
}

func (r *WizardResource) setWizardStateWatchError(err error) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if r.stateClosed {
			return
		}
		r.stateWatchErr = err
		broadcast()
	})
}

func (r *WizardResource) snapshotWizardStateWatchLocked() *wizardStateWatchSnapshot {
	if r.stateWatchErr != nil {
		return &wizardStateWatchSnapshot{err: r.stateWatchErr}
	}
	return &wizardStateWatchSnapshot{state: r.state.CloneVT()}
}

func wizardStateWatchSnapshotsEqual(a, b *wizardStateWatchSnapshot) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.err != nil || b.err != nil {
		if a.err == nil || b.err == nil {
			return false
		}
		return a.err.Error() == b.err.Error()
	}
	if a.state == nil || b.state == nil {
		return a.state == b.state
	}
	return a.state.EqualVT(b.state)
}

// UpdateWizardState updates the wizard block state.
func (r *WizardResource) UpdateWizardState(ctx context.Context, req *UpdateWizardStateRequest) (*UpdateWizardStateResponse, error) {
	var updated *WizardState
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		updated = r.state.CloneVT()
	})

	if req.GetStep() >= 0 {
		updated.Step = req.GetStep()
	}
	if req.GetName() != "" {
		updated.Name = req.GetName()
	}
	if req.GetHasConfigData() {
		updated.ConfigData = req.GetConfigData()
	}

	rev, err := r.persistState(ctx, updated)
	if err != nil {
		return nil, err
	}

	r.setWizardStateWatchState(updated, rev)

	return &UpdateWizardStateResponse{State: updated.CloneVT()}, nil
}

// CompareAndSetConfigData atomically persists one observed-to-desired config transition.
func (r *WizardResource) CompareAndSetConfigData(
	ctx context.Context,
	req *CompareAndSetConfigDataRequest,
) (*CompareAndSetConfigDataResponse, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		state, rev, applied, err := r.compareAndSetConfigData(ctx, req)
		if errors.Is(err, coord.ErrStaleGeneration) {
			continue
		}
		if err != nil {
			return nil, err
		}
		r.setWizardStateWatchState(state, rev)
		return &CompareAndSetConfigDataResponse{
			State:   state.CloneVT(),
			Applied: applied,
		}, nil
	}
}

func (r *WizardResource) compareAndSetConfigData(
	ctx context.Context,
	req *CompareAndSetConfigDataRequest,
) (*WizardState, uint64, bool, error) {
	wtx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, 0, false, err
	}
	defer wtx.Discard()
	writeState, found, err := wtx.GetObject(ctx, r.objKey)
	if err != nil {
		return nil, 0, false, err
	}
	if !found {
		return nil, 0, false, world.ErrObjectNotFound
	}
	current, err := r.readWizardWorldState(ctx, writeState)
	if err != nil {
		return nil, 0, false, err
	}
	if !bytes.Equal(current.GetConfigData(), req.GetExpectedConfigData()) {
		_, rev, err := writeState.GetRootRef(ctx)
		return current, rev, false, err
	}
	current.ConfigData = append([]byte(nil), req.GetConfigData()...)
	_, _, err = world.AccessObjectState(ctx, writeState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(current, true)
		return nil
	})
	if err != nil {
		return nil, 0, false, err
	}
	_, rev, err := writeState.GetRootRef(ctx)
	if err != nil {
		return nil, 0, false, err
	}
	if err := wtx.Commit(ctx); err != nil {
		return nil, 0, false, err
	}
	return current, rev, true, nil
}

// StartGitClone starts the Git repository clone workflow for this wizard.
func (r *WizardResource) StartGitClone(ctx context.Context, req *StartGitCloneRequest) (*StartGitCloneResponse, error) {
	if req.GetObjectKey() == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "object_key")
	}
	if req.GetName() == "" {
		return nil, errors.New("name is required")
	}
	if len(req.GetConfigData()) == 0 {
		return nil, errors.New("config_data is required")
	}

	var progress *GitCloneProgress
	var cloneReq *StartGitCloneRequest
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if r.cloneProgress.GetState() == GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_RUNNING {
			return
		}
		cloneReq = req.CloneVT()
		r.cloneProgress = &GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_RUNNING,
			Message:   "Starting clone...",
			ObjectKey: req.GetObjectKey(),
		}
		progress = r.cloneProgress.CloneVT()
		broadcast()
	})
	if cloneReq == nil {
		return nil, errors.New("git clone already running")
	}

	r.cloneRoutine.SetRoutine(func(runCtx context.Context) error {
		r.runGitClone(runCtx, cloneReq)
		return nil
	})

	return &StartGitCloneResponse{Progress: progress}, nil
}

// WatchGitCloneProgress streams Git clone progress for this wizard resource.
func (r *WizardResource) WatchGitCloneProgress(_ *WatchGitCloneProgressRequest, strm SRPCWizardResourceService_WatchGitCloneProgressStream) error {
	err := broadcast.WatchBroadcastVT(
		strm.Context(),
		&r.bcast,
		func() *GitCloneProgress {
			return r.cloneProgress.CloneVT()
		},
		func(progress *GitCloneProgress) error {
			if err := strm.Send(&WatchGitCloneProgressResponse{Progress: progress}); err != nil {
				return err
			}
			switch progress.GetState() {
			case GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_DONE,
				GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED:
				return errGitCloneProgressComplete
			default:
				return nil
			}
		},
	)
	if err == errGitCloneProgressComplete {
		return nil
	}
	return err
}

func (r *WizardResource) replaceSpaceIndexIfWizardIsCurrent(
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
) error {
	settings, _, err := space_world.LookupSpaceSettings(ctx, ws)
	if err != nil {
		return err
	}
	if settings != nil {
		settings = settings.CloneVT()
	}
	if settings == nil {
		settings = &space_world.SpaceSettings{}
	}
	if space_uri.ParseObjectURI(settings.GetIndexPath()).ObjectKey != r.objKey {
		return nil
	}
	settings.IndexPath = objectKey
	_, _, err = world.AccessWorldObject(
		ctx,
		ws,
		space_world.SpaceSettingsObjectKey,
		true,
		func(bcs *block.Cursor) error {
			bcs.SetBlock(settings.CloneVT(), true)
			return nil
		},
	)
	if err != nil {
		return err
	}
	return world_types.SetObjectType(
		ctx,
		ws,
		space_world.SpaceSettingsObjectKey,
		space_world.SpaceSettingsBlockType.GetBlockTypeID(),
	)
}

func (r *WizardResource) setGitCloneProgress(progress *GitCloneProgress) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.cloneProgress = progress.CloneVT()
		broadcast()
	})
}

// persistState writes the wizard state to the world via a write transaction.
func (r *WizardResource) persistState(ctx context.Context, state *WizardState) (uint64, error) {
	wtx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return 0, err
	}
	writeState, found, err := wtx.GetObject(ctx, r.objKey)
	if err != nil {
		wtx.Discard()
		return 0, err
	}
	if !found {
		wtx.Discard()
		return 0, world.ErrObjectNotFound
	}
	_, _, err = world.AccessObjectState(ctx, writeState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		wtx.Discard()
		return 0, err
	}
	_, rev, err := writeState.GetRootRef(ctx)
	if err != nil {
		wtx.Discard()
		return 0, err
	}
	if err := wtx.Commit(ctx); err != nil {
		return 0, err
	}
	return rev, nil
}

var _ SRPCWizardResourceServiceServer = (*WizardResource)(nil)

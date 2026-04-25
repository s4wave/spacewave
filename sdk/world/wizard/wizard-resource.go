package s4wave_wizard

import (
	"context"
	"strings"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// WizardResource implements the WizardResourceService SRPC interface.
type WizardResource struct {
	ws            world.WorldState
	engine        world.Engine
	objKey        string
	ctxCancel     context.CancelFunc
	cloneRoutine  *routine.RoutineContainer
	state         *WizardState
	cloneProgress *GitCloneProgress
	bcast         broadcast.Broadcast
	mux           srpc.Mux
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
	r.ctxCancel()
}

// WatchWizardState streams wizard state changes.
func (r *WizardResource) WatchWizardState(_ *WatchWizardStateRequest, strm SRPCWizardResourceService_WatchWizardStateStream) error {
	ctx := strm.Context()

	objState, found, err := r.ws.GetObject(ctx, r.objKey)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	var lastSent *WizardState
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		_, rev, err := objState.GetRootRef(ctx)
		if err != nil {
			return err
		}

		var state *WizardState
		_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
			var uerr error
			state, uerr = UnmarshalWizardState(ctx, bcs)
			return uerr
		})
		if err != nil {
			return err
		}
		if state == nil {
			state = &WizardState{}
		}

		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.state = state.CloneVT()
			broadcast()
		})

		if lastSent == nil || !state.EqualVT(lastSent) {
			if serr := strm.Send(&WatchWizardStateResponse{State: state.CloneVT()}); serr != nil {
				return serr
			}
			lastSent = state
		}

		_, err = objState.WaitRev(ctx, rev+1, false)
		if err != nil {
			return err
		}
	}
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

	if err := r.persistState(ctx, updated); err != nil {
		return nil, err
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.state = updated
		broadcast()
	})

	return &UpdateWizardStateResponse{State: updated.CloneVT()}, nil
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
	ctx := strm.Context()

	var lastSent *GitCloneProgress
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var waitCh <-chan struct{}
		var progress *GitCloneProgress
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			progress = r.cloneProgress.CloneVT()
		})

		if lastSent == nil || !progress.EqualVT(lastSent) {
			if err := strm.Send(&WatchGitCloneProgressResponse{Progress: progress}); err != nil {
				return err
			}
			lastSent = progress
		}

		if progress.GetState() == GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_DONE ||
			progress.GetState() == GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

func (r *WizardResource) runGitClone(ctx context.Context, req *StartGitCloneRequest) {
	op := &s4wave_git.CreateGitRepoWizardOp{}
	if err := op.UnmarshalVT(req.GetConfigData()); err != nil {
		r.setGitCloneProgress(&GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
			Message:   "Clone configuration is invalid.",
			Error:     err.Error(),
			ObjectKey: req.GetObjectKey(),
		})
		return
	}
	if err := op.Validate(); err != nil {
		r.setGitCloneProgress(&GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
			Message:   "Clone configuration is invalid.",
			Error:     err.Error(),
			ObjectKey: req.GetObjectKey(),
		})
		return
	}
	if !op.GetClone() {
		r.setGitCloneProgress(&GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
			Message:   "Clone configuration is invalid.",
			Error:     "clone must be true",
			ObjectKey: req.GetObjectKey(),
		})
		return
	}

	sender, err := confparse.ParsePeerID(req.GetOpSender())
	if err != nil {
		r.setGitCloneProgress(&GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
			Message:   "Clone sender is invalid.",
			Error:     err.Error(),
			ObjectKey: req.GetObjectKey(),
		})
		return
	}

	ws := world.NewEngineWorldState(r.engine, true)
	ts := op.GetTimestamp()
	if ts == nil {
		ts = timestamppb.Now()
	}
	repoRef, err := s4wave_git.CloneGitRepoToRef(
		ctx,
		r.engine,
		op.GetCloneOpts(),
		nil,
		&gitCloneProgressWriter{resource: r, objectKey: req.GetObjectKey()},
	)
	if err != nil {
		r.setGitCloneProgress(&GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
			Message:   "Clone failed.",
			Error:     err.Error(),
			ObjectKey: req.GetObjectKey(),
		})
		return
	}

	wtx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		r.setGitCloneProgress(&GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
			Message:   "Repository was cloned, but publish failed.",
			Error:     err.Error(),
			ObjectKey: req.GetObjectKey(),
		})
		return
	}
	defer wtx.Discard()
	initOp := git_world.NewGitInitOp(req.GetObjectKey(), repoRef, true, nil, ts)
	_, _, err = wtx.ApplyWorldOp(ctx, initOp, sender)
	if err != nil {
		r.setGitCloneProgress(&GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
			Message:   "Repository was cloned, but publish failed.",
			Error:     err.Error(),
			ObjectKey: req.GetObjectKey(),
		})
		return
	}
	if err := wtx.Commit(ctx); err != nil {
		r.setGitCloneProgress(&GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
			Message:   "Repository was cloned, but publish failed.",
			Error:     err.Error(),
			ObjectKey: req.GetObjectKey(),
		})
		return
	}

	if err := r.setSpaceIndex(ctx, ws, req.GetObjectKey()); err != nil {
		r.setGitCloneProgress(&GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
			Message:   "Repository was cloned, but space index update failed.",
			Error:     err.Error(),
			ObjectKey: req.GetObjectKey(),
		})
		return
	}

	if _, err := ws.DeleteObject(ctx, r.objKey); err != nil {
		r.setGitCloneProgress(&GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
			Message:   "Repository was cloned, but wizard cleanup failed.",
			Error:     err.Error(),
			ObjectKey: req.GetObjectKey(),
		})
		return
	}

	r.setGitCloneProgress(&GitCloneProgress{
		State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_DONE,
		Message:   "Repository cloned.",
		ObjectKey: req.GetObjectKey(),
	})
}

func (r *WizardResource) setSpaceIndex(
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
func (r *WizardResource) persistState(ctx context.Context, state *WizardState) error {
	wtx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	writeState, found, err := wtx.GetObject(ctx, r.objKey)
	if err != nil {
		wtx.Discard()
		return err
	}
	if !found {
		wtx.Discard()
		return world.ErrObjectNotFound
	}
	_, _, err = world.AccessObjectState(ctx, writeState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		wtx.Discard()
		return err
	}
	return wtx.Commit(ctx)
}

type gitCloneProgressWriter struct {
	resource  *WizardResource
	objectKey string
}

func (w *gitCloneProgressWriter) Write(p []byte) (int, error) {
	message := strings.TrimSpace(string(p))
	if message != "" {
		w.resource.setGitCloneProgress(&GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_RUNNING,
			Message:   message,
			ObjectKey: w.objectKey,
		})
	}
	return len(p), nil
}

// _ is a type assertion
var _ SRPCWizardResourceServiceServer = (*WizardResource)(nil)

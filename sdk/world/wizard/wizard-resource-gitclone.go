//go:build !tinygo

package s4wave_wizard

import (
	"context"
	"strings"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/util/confparse"
)

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

	if err := r.replaceSpaceIndexIfWizardIsCurrent(ctx, ws, req.GetObjectKey()); err != nil {
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

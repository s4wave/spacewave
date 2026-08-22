//go:build !tinygo

package s4wave_wizard

import (
	"context"
	"strings"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// errGitCloneRequired reports a clone config that did not request a clone.
var errGitCloneRequired = errors.New("clone must be true")

// failGitClone reports a failed clone with the given user-facing message and
// optional error detail.
func (r *WizardResource) failGitClone(objectKey, message string, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	r.setGitCloneProgress(&GitCloneProgress{
		State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
		Message:   message,
		Error:     errMsg,
		ObjectKey: objectKey,
	})
}

func (r *WizardResource) runGitClone(ctx context.Context, req *StartGitCloneRequest) {
	op := &s4wave_git.CreateGitRepoWizardOp{}
	if err := op.UnmarshalVT(req.GetConfigData()); err != nil {
		r.failGitClone(req.GetObjectKey(), "Clone configuration is invalid.", err)
		return
	}
	if err := op.Validate(); err != nil {
		r.failGitClone(req.GetObjectKey(), "Clone configuration is invalid.", err)
		return
	}
	if !op.GetClone() {
		r.failGitClone(req.GetObjectKey(), "Clone configuration is invalid.", errGitCloneRequired)
		return
	}

	sender, err := confparse.ParsePeerID(req.GetOpSender())
	if err != nil {
		r.failGitClone(req.GetObjectKey(), "Clone sender is invalid.", err)
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
		r.failGitClone(req.GetObjectKey(), "Clone failed.", err)
		return
	}

	wtx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		r.failGitClone(req.GetObjectKey(), "Repository was cloned, but publish failed.", err)
		return
	}
	defer wtx.Discard()
	initOp := git_world.NewGitInitOp(req.GetObjectKey(), repoRef, true, nil, ts)
	_, _, err = wtx.ApplyWorldOp(ctx, initOp, sender)
	if err != nil {
		r.failGitClone(req.GetObjectKey(), "Repository was cloned, but publish failed.", err)
		return
	}
	if err := wtx.Commit(ctx); err != nil {
		r.failGitClone(req.GetObjectKey(), "Repository was cloned, but publish failed.", err)
		return
	}

	if err := r.replaceSpaceIndexIfWizardIsCurrent(ctx, ws, req.GetObjectKey()); err != nil {
		r.failGitClone(req.GetObjectKey(), "Repository was cloned, but space index update failed.", err)
		return
	}

	if _, err := ws.DeleteObject(ctx, r.objKey); err != nil {
		r.failGitClone(req.GetObjectKey(), "Repository was cloned, but wizard cleanup failed.", err)
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

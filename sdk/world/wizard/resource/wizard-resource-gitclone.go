//go:build !tinygo

package wizard_resource

import (
	"context"
	"strings"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	"github.com/s4wave/spacewave/db/bucket"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
	wizard "github.com/s4wave/spacewave/sdk/world/wizard"
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
	r.setGitCloneProgress(&wizard.GitCloneProgress{
		State:     wizard.GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
		Message:   message,
		Error:     errMsg,
		ObjectKey: objectKey,
	})
}

func (r *WizardResource) runGitClone(ctx context.Context, req *wizard.StartGitCloneRequest) {
	// Decode and validate the requested repository clone.
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

	// Resolve the peer that will publish the cloned repository.
	sender, err := confparse.ParsePeerID(req.GetOpSender())
	if err != nil {
		r.failGitClone(req.GetObjectKey(), "Clone sender is invalid.", err)
		return
	}

	// Prepare the repository timestamp.
	ts := op.GetTimestamp()
	if ts == nil {
		ts = timestamppb.Now()
	}

	// Clone the repository through a stage held until the publish commits,
	// publishing progress to the wizard.
	stage, err := r.engine.StageWorldState(ctx)
	if err != nil {
		r.failGitClone(req.GetObjectKey(), "Clone failed.", err)
		return
	}
	defer stage.Release()
	repoRef, err := s4wave_git.CloneGitRepoToRef(
		ctx,
		stage,
		op.GetCloneOpts(),
		nil,
		&gitCloneProgressWriter{resource: r, objectKey: req.GetObjectKey()},
	)
	if err != nil {
		r.failGitClone(req.GetObjectKey(), "Clone failed.", err)
		return
	}

	// Publish the staged clone and retire the wizard together.
	if err := r.publishGitClone(ctx, req.GetObjectKey(), repoRef, ts, sender); err != nil {
		r.failGitClone(req.GetObjectKey(), "Repository was cloned, but publish failed.", err)
		return
	}

	// Notify clone watchers that the repository is ready.
	r.setGitCloneProgress(&wizard.GitCloneProgress{
		State:     wizard.GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_DONE,
		Message:   "Repository cloned.",
		ObjectKey: req.GetObjectKey(),
	})
}

// publishGitClone creates the repository object at objectKey from the staged
// clone at repoRef, points the Space index at it while the index names this
// wizard, and deletes the wizard, all in one World transaction.
//
// Invariant: the repository object, the index change and the wizard deletion
// commit together or not at all. A failure leaves no repository object and
// the wizard present, so a retry clones and publishes once, never beside an
// earlier repository.
//
// The transaction holds the World writer only for local reads and writes:
// validating the staged repository root, creating the object and its type,
// the Space settings write and the delete. The network clone finished before
// it opened. The caller holds the stage until this returns.
func (r *WizardResource) publishGitClone(
	ctx context.Context,
	objectKey string,
	repoRef *bucket.ObjectRef,
	ts *timestamppb.Timestamp,
	sender peer.ID,
) error {
	// Open the write transaction; a return before Commit discards it.
	wtx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer wtx.Discard()

	// Create the repository object over the staged clone.
	initOp := git_world.NewGitInitOp(objectKey, repoRef, true, nil, ts)
	if _, _, err := wtx.ApplyWorldOp(ctx, initOp, sender); err != nil {
		return err
	}

	// Point the Space index at the repository when this wizard is current.
	if err := r.replaceSpaceIndexIfWizardIsCurrent(ctx, wtx, objectKey); err != nil {
		return errors.Wrap(err, "update space index")
	}

	// Delete the wizard that the repository replaces.
	if _, err := wtx.DeleteObject(ctx, r.objKey); err != nil {
		return errors.Wrap(err, "delete wizard")
	}

	return wtx.Commit(ctx)
}

// gitCloneProgressWriter forwards git clone progress output into the
// wizard's clone progress state.
type gitCloneProgressWriter struct {
	resource  *WizardResource
	objectKey string
}

// Write publishes each non-empty line of clone progress as the running
// state and reports every byte as written, so progress never fails the clone.
func (w *gitCloneProgressWriter) Write(p []byte) (int, error) {
	message := strings.TrimSpace(string(p))
	if message != "" {
		w.resource.setGitCloneProgress(&wizard.GitCloneProgress{
			State:     wizard.GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_RUNNING,
			Message:   message,
			ObjectKey: w.objectKey,
		})
	}
	return len(p), nil
}

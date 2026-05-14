//go:build tinygo

package s4wave_wizard

import "context"

func (r *WizardResource) runGitClone(ctx context.Context, req *StartGitCloneRequest) {
	if err := ctx.Err(); err != nil {
		r.setGitCloneProgress(&GitCloneProgress{
			State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
			Message:   "Clone canceled.",
			Error:     err.Error(),
			ObjectKey: req.GetObjectKey(),
		})
		return
	}
	r.setGitCloneProgress(&GitCloneProgress{
		State:     GitCloneProgressState_GIT_CLONE_PROGRESS_STATE_FAILED,
		Message:   "Git clone is not available in this browser build.",
		ObjectKey: req.GetObjectKey(),
	})
}

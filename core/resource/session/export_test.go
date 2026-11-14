package resource_session

import "context"

// WaitTransferDone waits for the transfer goroutine to finish completely
// (including the source account delete of a Merge or Migrate).
func (r *SessionResource) WaitTransferDone(ctx context.Context) error {
	// Wait for the transfer routine to exit, ignoring cancellation after it has stopped.
	r.transferMgr.mtx.Lock()
	rc := r.transferMgr.rc
	r.transferMgr.mtx.Unlock()
	if rc == nil {
		return nil
	}
	if err := rc.WaitExited(ctx, true, nil); err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

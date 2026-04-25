package resource_session

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	"github.com/s4wave/spacewave/core/session"
)

// ReadLinkedCloudAccountID exports readLinkedCloudAccountID for testing.
func ReadLinkedCloudAccountID(ctx context.Context, b bus.Bus, entry *session.SessionListEntry, source provider_transfer.TransferSource) (string, error) {
	return readLinkedCloudAccountID(ctx, b, entry, source)
}

// WaitTransferDone waits for the transfer goroutine to finish completely
// (including post-transfer cleanup like session deletion).
func (r *SessionResource) WaitTransferDone(ctx context.Context) error {
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

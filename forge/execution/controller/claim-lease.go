package execution_controller

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_transaction "github.com/s4wave/spacewave/forge/execution/tx"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/net/peer"
)

// foreignClaim is a claim held by another controller, observed on behalf of peerID.
type foreignClaim struct {
	// peerID is the peer the observer reclaims the Execution as.
	peerID peer.ID
	// claim is the observed claim, including its lease.
	claim *forge_execution.Claim
}

// compareForeignClaim reports whether two observed claims are identical.
func compareForeignClaim(a, b *foreignClaim) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.peerID == b.peerID && a.claim.EqualVT(b.claim)
}

// reclaimExpiredClaim waits for the lease of the observed claim to expire, then
// reclaims the Execution. A renewal or a competing reclaim changes the observed
// claim, which restarts the routine with the new state.
func (c *Controller) reclaimExpiredClaim(ctx context.Context, fc *foreignClaim) error {
	// Wait once for the known lease deadline; the watch replaces renewed claims.
	timer := time.NewTimer(time.Until(fc.claim.GetLeaseExpiresAt().AsTime()))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Canceled
	case <-timer.C:
	}

	// Replace the expired claim. The epoch advances once, so a late write from the
	// previous holder and a competing reclaim are both rejected.
	c.le.
		WithField("claim-id", fc.claim.GetClaimId()).
		WithField("claim-epoch", fc.claim.GetEpoch()).
		Info("execution claim lease expired, reclaiming")
	now := time.Now()
	var placement *forge_worker.Placement
	if c.conf.GetWorkerObjectKey() != "" {
		placement = &forge_worker.Placement{
			WorkerObjectKey: c.conf.GetWorkerObjectKey(),
			PeerId:          fc.peerID.String(),
		}
	}
	txd := execution_transaction.NewTxReclaim(
		c.conf.GetObjectKey(),
		fc.peerID,
		c.claimID,
		fc.claim.GetEpoch(),
		now,
		now.Add(c.claimLease),
		placement,
	)

	// Let the World transaction decide whether custody still needs transfer.
	_, _, err := c.ws.ApplyWorldOp(ctx, txd, fc.peerID)
	var liveErr *execution_transaction.ClaimLiveError
	if execution_transaction.IsClaimFenced(err) || errors.As(err, &liveErr) {
		// Another controller renewed or reclaimed first; the watch reports it.
		c.le.WithError(err).Debug("execution claim changed before reclaim")
		return nil
	}
	return err
}

// applyClaimTx applies a claim transaction to the Execution object.
func (c *Controller) applyClaimTx(ctx context.Context, txd *execution_transaction.Tx, sender peer.ID) error {
	// Look up the Execution object.
	obj, err := world.MustGetObject(ctx, c.ws, c.conf.GetObjectKey())
	if err != nil {
		return err
	}
	defer world.ReleaseObjectState(obj)

	// Apply the transaction as the sender.
	_, _, err = obj.ApplyObjectOp(ctx, txd, sender)
	return err
}

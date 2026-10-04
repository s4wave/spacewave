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

// reclaimableClaim is a claim this controller cannot run under, observed on
// behalf of peerID: another controller's claim, or its own lapsed one.
type reclaimableClaim struct {
	// peerID is the peer the observer reclaims the Execution as.
	peerID peer.ID
	// claim is the observed claim, including its lease.
	claim *forge_execution.Claim
}

// compareReclaimableClaim reports whether two observed claims are identical.
func compareReclaimableClaim(a, b *reclaimableClaim) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.peerID == b.peerID && a.claim.EqualVT(b.claim)
}

// reclaimExpiredClaim waits for the lease of the observed claim to expire, then
// reclaims the Execution. A renewal or a competing reclaim changes the observed
// claim, which restarts the routine with the new state.
func (c *Controller) reclaimExpiredClaim(ctx context.Context, rc *reclaimableClaim) error {
	// Wait once for the known lease deadline; the watch replaces renewed claims.
	timer := time.NewTimer(time.Until(rc.claim.GetLeaseExpiresAt().AsTime()))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Canceled
	case <-timer.C:
	}

	// Replace the expired claim. The epoch advances once, so a late write from the
	// previous holder and a competing reclaim are both rejected.
	c.le.
		WithField("claim-id", rc.claim.GetClaimId()).
		WithField("claim-epoch", rc.claim.GetEpoch()).
		Info("execution claim lease expired, reclaiming")
	now := time.Now()
	var placement *forge_worker.Placement
	if c.conf.GetWorkerObjectKey() != "" {
		placement = &forge_worker.Placement{
			WorkerObjectKey: c.conf.GetWorkerObjectKey(),
			PeerId:          rc.peerID.String(),
		}
	}
	txd := execution_transaction.NewTxReclaim(
		c.conf.GetObjectKey(),
		rc.peerID,
		c.claimID,
		rc.claim.GetEpoch(),
		now,
		now.Add(c.claimLease),
		placement,
	)

	// Let the World transaction decide whether custody still needs transfer.
	_, _, err := c.ws.ApplyWorldOp(ctx, txd, rc.peerID)
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

package execution_tx

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// NewTxReclaim constructs a RECLAIM transaction.
//
// observedAt is when the sender saw the claim lease expire. The new claim stays
// live until leaseExpiresAt unless its holder renews it.
func NewTxReclaim(
	executionObjectKey string,
	peerID peer.ID,
	claimID string,
	expectedClaimEpoch uint64,
	observedAt, leaseExpiresAt time.Time,
	placement *forge_worker.Placement,
) *Tx {
	return &Tx{
		TxType: TxType_TxType_RECLAIM,
		TxReclaim: &TxReclaim{
			ExecutionObjectKey: executionObjectKey,
			PeerId:             peerID.String(),
			ClaimId:            claimID,
			ExpectedClaimEpoch: expectedClaimEpoch,
			ObservedAt:         timestamp.New(observedAt),
			LeaseExpiresAt:     timestamp.New(leaseExpiresAt),
			Placement:          placement.CloneVT(),
		},
	}
}

// NewTxReclaimTxn constructs a new RECLAIM transaction.
func NewTxReclaimTxn() Transaction {
	return &TxReclaim{}
}

// GetTxType returns the type of transaction this is.
func (t *TxReclaim) GetTxType() TxType {
	return TxType_TxType_RECLAIM
}

// Validate performs a cursory check of the transaction.
func (t *TxReclaim) Validate() error {
	// Require the Execution key and the peer taking custody.
	if t.GetExecutionObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if len(t.GetPeerId()) == 0 {
		return peer.ErrEmptyPeerID
	}
	if _, err := t.ParsePeerID(); err != nil {
		return err
	}

	// Require a claim identity, expected epoch, and a future lease.
	if t.GetClaimId() == "" {
		return errors.New("claim_id cannot be empty")
	}
	if t.GetExpectedClaimEpoch() == 0 {
		return errors.New("expected claim epoch cannot be zero")
	}
	if t.GetObservedAt() == nil {
		return errors.New("observed_at cannot be empty")
	}
	if !t.GetLeaseExpiresAt().AsTime().After(t.GetObservedAt().AsTime()) {
		return errors.New("lease_expires_at must be after observed_at")
	}

	// Require the requested placement to identify the reclaiming peer.
	if placement := t.GetPlacement(); placement != nil {
		if err := placement.Validate(); err != nil {
			return err
		}
		if placement.GetPeerId() != t.GetPeerId() {
			return errors.New("reclaim peer_id does not match placement")
		}
	}
	return nil
}

// applyWorldOp replaces the claim, peer, placement and Worker edge together.
// World replay uses the same epoch and expiry checks as the initial operation.
func (t *TxReclaim) applyWorldOp(ctx context.Context, ws world.WorldState, sender peer.ID) error {
	// Require the requested Worker to carry the reclaiming peer.
	if err := t.Validate(); err != nil {
		return err
	}
	if placement := t.GetPlacement(); placement != nil {
		if err := placement.ValidateLinked(ctx, ws); err != nil {
			return err
		}
	}

	// Replace the Execution only if the observed claim is still expired.
	objKey := t.GetExecutionObjectKey()
	_, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(cursor *block.Cursor) error {
		// Decode the current claim and apply the fenced custody transfer.
		root, err := forge_execution.UnmarshalExecution(ctx, cursor)
		if err != nil {
			return err
		}
		return t.ExecuteTx(ctx, sender, cursor, root)
	})
	if err != nil {
		return err
	}

	// Replace the sole Worker relationship with the committed placement.
	quads, err := ws.LookupGraphQuads(ctx, forge_execution.NewExecutionToWorkerQuad(objKey, ""), 0)
	if err != nil {
		return err
	}
	for _, q := range quads {
		if err := ws.DeleteGraphQuad(ctx, q); err != nil {
			return err
		}
	}
	if placement := t.GetPlacement(); placement != nil {
		if err := ws.SetGraphQuad(ctx, forge_execution.NewExecutionToWorkerQuad(objKey, placement.GetWorkerObjectKey())); err != nil {
			return err
		}
	}

	// Notify the reclaiming Worker's existing keypair watch of its new custody.
	peerID, err := t.ParsePeerID()
	if err != nil {
		return err
	}
	_, _, err = identity_world.LinkObjectToKeypair(ctx, ws, sender, objKey, peerID, "", nil)
	return err
}

// ExecuteTx executes the transaction against the execution instance.
func (t *TxReclaim) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	exCursor *block.Cursor,
	root *forge_execution.Execution,
) error {
	// Require the reclaim sender, execution state, and current claim epoch.
	txPeerID, err := t.ParsePeerID()
	if err != nil {
		return err
	}
	if len(txPeerID) == 0 {
		return peer.ErrEmptyPeerID
	}
	if len(sender) != 0 && sender != txPeerID {
		return errors.Errorf(
			"tx body peer id %s must match sender %s",
			txPeerID.String(), sender.String(),
		)
	}

	// Require an active Execution and the exact claim being replaced. The new
	// claim may reuse the replaced claim's id: the epoch fences its old holder.
	if err := root.GetExecutionState().EnsureMatches(
		forge_execution.State_ExecutionState_RUNNING,
		forge_execution.State_ExecutionState_CANCELING,
	); err != nil {
		return err
	}
	if err := checkClaimEpoch(root.GetClaim().GetEpoch(), t.GetExpectedClaimEpoch()); err != nil {
		return err
	}

	// Require the sender to have observed the current lease expire.
	if !root.GetClaim().LeaseExpired(t.GetObservedAt().AsTime()) {
		return &ClaimLiveError{
			ClaimID:        root.GetClaim().GetClaimId(),
			Epoch:          root.GetClaim().GetEpoch(),
			LeaseExpiresAt: root.GetClaim().GetLeaseExpiresAt().AsTime(),
		}
	}

	// Preserve placed custody by requiring a linked replacement Worker.
	if root.GetPlacement() != nil && t.GetPlacement() == nil {
		return errors.New("reclaim of a placed execution requires placement")
	}

	// Replace the executor, placement, claim, and waiting plugin in one root.
	claimEpoch := root.GetClaim().GetEpoch() + 1
	if claimEpoch == 0 {
		return errors.New("execution claim epoch overflow")
	}
	root.Claim = &forge_execution.Claim{
		ClaimId:        t.GetClaimId(),
		Epoch:          claimEpoch,
		LeaseExpiresAt: t.GetLeaseExpiresAt(),
	}
	root.PeerId = t.GetPeerId()
	root.Placement = t.GetPlacement().CloneVT()
	root.WaitingPluginId = ""
	exCursor.SetBlock(root, true)
	return root.Validate()
}

// ParsePeerID parses the peer ID field.
func (t *TxReclaim) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(t.GetPeerId())
}

// _ is a type assertion
var _ Transaction = (*TxReclaim)(nil)

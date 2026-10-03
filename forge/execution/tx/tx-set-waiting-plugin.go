package execution_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxSetWaitingPlugin constructs a claim-fenced plugin wait update.
func NewTxSetWaitingPlugin(pluginID string, claim *forge_execution.Claim) *Tx {
	return &Tx{
		TxType: TxType_TxType_SET_WAITING_PLUGIN,
		TxSetWaitingPlugin: &TxSetWaitingPlugin{
			PluginId:   pluginID,
			ClaimId:    claim.GetClaimId(),
			ClaimEpoch: claim.GetEpoch(),
		},
	}
}

// GetTxType returns the plugin wait transaction type.
func (t *TxSetWaitingPlugin) GetTxType() TxType { return TxType_TxType_SET_WAITING_PLUGIN }

// Validate requires a current execution claim.
func (t *TxSetWaitingPlugin) Validate() error {
	if t.GetClaimId() == "" {
		return errors.New("claim_id cannot be empty")
	}
	if t.GetClaimEpoch() == 0 {
		return &StaleClaimEpochError{}
	}
	return nil
}

// ExecuteTx writes the waiting plugin ID while the claimed execution is active.
func (t *TxSetWaitingPlugin) ExecuteTx(ctx context.Context, sender peer.ID, exCursor *block.Cursor, root *forge_execution.Execution) error {
	// Require the sender and claim to match an active execution.
	if len(sender) != 0 {
		if err := root.CheckPeerID(sender); err != nil {
			return err
		}
	}
	if err := checkClaim(root.GetClaim(), t.GetClaimId(), t.GetClaimEpoch()); err != nil {
		return err
	}
	if err := root.GetExecutionState().EnsureMatches(forge_execution.State_ExecutionState_RUNNING, forge_execution.State_ExecutionState_CANCELING); err != nil {
		return err
	}

	// Persist the plugin awaited by the claimed execution.
	root.WaitingPluginId = t.GetPluginId()
	exCursor.SetBlock(root, true)
	return nil
}

// _ is a type assertion
var _ Transaction = (*TxSetWaitingPlugin)(nil)

package pass_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_pass "github.com/aperturerobotics/forge/pass"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// NewTxStart constructs a new START transaction.
func NewTxStart(replicas uint32, execSpecs []*ExecSpec) *Tx {
	return &Tx{
		TxType: TxType_TxType_START,
		TxStart: &TxStart{
			Replicas:  replicas,
			ExecSpecs: execSpecs,
		},
	}
}

// NewTxStartTxn constructs a new START transaction.
func NewTxStartTxn() Transaction {
	return &TxStart{}
}

// GetTxType returns the type of transaction this is.
func (t *TxStart) GetTxType() TxType {
	return TxType_TxType_START
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxStart) Validate() error {
	replicaCount := t.GetReplicaCount()
	execSpecs := t.GetExecSpecs()
	if len(execSpecs) != 0 && len(execSpecs) != replicaCount {
		return errors.Errorf(
			"exec spec count %d must match replica count %d",
			len(execSpecs), replicaCount,
		)
	}
	seenIDs := make(map[string]struct{})
	for i, spec := range execSpecs {
		if err := spec.Validate(); err != nil {
			return errors.Wrapf(err, "exec_specs[%d]", i)
		}
		if pid := spec.GetPeerId(); pid != "" {
			if _, ok := seenIDs[pid]; ok {
				return errors.Errorf(
					"exec_specs[%d]: peer id %s appears multiple times",
					i,
					pid,
				)
			}
			seenIDs[pid] = struct{}{}
		}
	}
	return nil
}

// GetReplicaCount returns the replica count.
func (t *TxStart) GetReplicaCount() int {
	v := int(t.GetReplicas())
	if v == 0 {
		v = 1
	}
	return v
}

// ExecuteTx executes the transaction against the execution instance.
func (t *TxStart) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	executorPeerID peer.ID,
	bcs *block.Cursor,
	root *forge_pass.Pass,
) error {
	// ensure PENDING
	passState := root.GetPassState()
	if passState != forge_pass.State_PassState_PENDING {
		return errors.Wrapf(
			forge_value.ErrUnknownState,
			"%s", passState.String(),
		)
	}

	// TODO

	// promote to RUNNING
	// root.PassState = forge_pass.State_PassState_RUNNING
	// exCursor.SetBlock(root, true)

	return errors.New("TODO exec TxStart in pass")
}

// _ is a type assertion
var _ Transaction = ((*TxStart)(nil))

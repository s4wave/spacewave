package pass_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxCancel constructs a cancellation transaction.
func NewTxCancel(objKey string, result *forge_value.Result) *Tx {
	return &Tx{
		PassObjectKey: objKey,
		TxType:        TxType_TxType_CANCEL,
		TxCancel: &TxCancel{
			Result: result,
		},
	}
}

// NewTxCancelTxn constructs a cancellation transaction payload.
func NewTxCancelTxn() Transaction {
	return &TxCancel{}
}

// GetTxType returns the type of transaction this is.
func (t *TxCancel) GetTxType() TxType {
	return TxType_TxType_CANCEL
}

// Validate checks that cancellation carries a canceled result.
func (t *TxCancel) Validate() error {
	if err := t.GetResult().Validate(); err != nil {
		return err
	}
	if !t.GetResult().GetCanceled() {
		return errors.New("cancel result must be canceled")
	}
	return nil
}

// ExecuteTx records a durable cancellation request.
func (t *TxCancel) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	sender peer.ID,
	objKey string,
	bcs *block.Cursor,
	root *forge_pass.Pass,
) error {
	switch state := root.GetPassState(); state {
	case forge_pass.State_PassState_PENDING,
		forge_pass.State_PassState_RUNNING,
		forge_pass.State_PassState_CHECKING:
		result := t.GetResult()
		if result == nil {
			result = forge_value.NewResultWithCanceled(nil)
		}
		result.FillFailError()
		root.PassState = forge_pass.State_PassState_CANCELING
		root.Result = result
	case forge_pass.State_PassState_CANCELING,
		forge_pass.State_PassState_COMPLETE:
		return nil
	default:
		return errors.Wrapf(forge_value.ErrUnknownState, "%s", state.String())
	}

	bcs.SetBlock(root, true)
	return root.Validate(false)
}

// _ is a type assertion
var _ Transaction = (*TxCancel)(nil)

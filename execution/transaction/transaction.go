package execution_transaction

import (
	"context"

	"github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/hydra/block"
	proto "github.com/golang/protobuf/proto"
)

// maxRequestBodyBytes is the maximum body size.
var maxRequestBodyBytes = int64(100 * 1024 * 1024)

// Transaction is an instance of a transaction object.
type Transaction interface {
	proto.Message

	// GetExecutionTransactionType returns the type of transaction this is.
	GetExecutionTransactionType() ExecutionTxType
	// Validate performs a cursory check of the transaction.
	// Note: this should not fetch network data.
	Validate() error
	// ExecuteTx executes the transaction against the execution instance.
	// txCursor should be located at the transaction.
	// exCursor should be located at the execution state root.
	// The transaction may be traversed via txCursor.
	// The result is written into exCursor.
	// The results will be saved if !dryRun.
	// If sysErr == true, tx is not marked invalid and will retry.
	ExecuteTx(
		ctx context.Context,
		txCursor *block.Cursor,
		exCursor *block.Cursor,
		root *forge_execution.Execution,
		dryRun bool,
	) (sysErr bool, err error)
}

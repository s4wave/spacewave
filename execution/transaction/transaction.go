package execution_transaction

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	proto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// maxRequestBodyBytes is the maximum body size.
var maxRequestBodyBytes = int64(100 * 1024 * 1024)

// ObjectOperationTypeID is the transaction object operation type id.
// Corresponds to a single *TransactionData object.
var ObjectOperationTypeID = "forge/execution/transaction"

// Transaction is an instance of a transaction object.
type Transaction interface {
	proto.Message

	// GetExecutionTransactionType returns the type of transaction this is.
	GetExecutionTransactionType() ExecutionTxType
	// Validate performs a cursory check of the transaction.
	// Note: this should not fetch network data.
	Validate() error
	// ExecuteTx executes the transaction against the execution instance.
	// exCursor should be located at the execution state root.
	// The result is written into exCursor.
	ExecuteTx(
		ctx context.Context,
		executorPeerID peer.ID,
		exCursor *block.Cursor,
		root *forge_execution.Execution,
	) error
}

// Validate checks the execution tx type is in range.
func (t ExecutionTxType) Validate() error {
	switch t {
	case ExecutionTxType_EXECUTION_TX_TYPE_START:
		return nil
	case ExecutionTxType_EXECUTION_TX_TYPE_SET_OUTPUTS:
		return nil
	case ExecutionTxType_EXECUTION_TX_TYPE_COMPLETE:
		return nil
	default:
		return errors.Errorf("unknown transaction type: %s", t.String())
	}
}

// transConst is the set of transaction constructors.
var transConst = make(map[ExecutionTxType]func() Transaction)

// addTransConst registers a transaction constructor.
func addTransConst(t ExecutionTxType, c func() Transaction) {
	transConst[t] = c
}

// UnknownTransactionTypeErr is a transaction type unknown error
type UnknownTransactionTypeErr struct {
	error
}

// NewUnknownTransactionTypeErr builds a new UnknownTransactionTypeErr
func NewUnknownTransactionTypeErr(txType ExecutionTxType) error {
	return &UnknownTransactionTypeErr{
		errors.Errorf("unknown transaction type: %s", txType.String()),
	}
}

// NewTransaction builds a new transaction by ID.
func NewTransaction(t ExecutionTxType) (Transaction, error) {
	tCon, ok := transConst[t]
	if !ok {
		return nil, NewUnknownTransactionTypeErr(t)
	}

	return tCon(), nil
}

// IsTransactionTypeKnown checks if a transaction type is known.
func IsTransactionTypeKnown(typ ExecutionTxType) bool {
	_, ok := transConst[typ]
	return ok
}

// UnmarshalTransaction unmarshals the encoded transaction.
func (d *ExecutionTxData) UnmarshalTransaction() (Transaction, error) {
	tx, err := NewTransaction(d.GetExecutionTxType())
	if err != nil {
		return nil, err
	}

	if err := proto.Unmarshal(d.GetTransactionBody(), tx); err != nil {
		return nil, err
	}

	return tx, nil
}

// NewTransactionData builds a new instance of a transaction data object.
func NewTransactionData(t Transaction) (*ExecutionTxData, error) {
	tData, err := proto.Marshal(t)
	if err != nil {
		return nil, err
	}

	return &ExecutionTxData{
		ExecutionTxType: t.GetExecutionTransactionType(),
		TransactionBody: tData,
	}, nil
}

// ByteSliceToTransactionData converts a byte slice block a ExecutionTxData.
// If blk is nil, returns nil, nil
// If the blk is already parsed to a MockWorldOp, returns the MockWorldOp.
func ByteSliceToTransactionData(blk block.Block) (*ExecutionTxData, error) {
	if blk == nil {
		return nil, nil
	}
	var out *ExecutionTxData
	nr, ok := blk.(*byteslice.ByteSlice)
	if ok && nr != nil {
		out = &ExecutionTxData{}
		if err := out.UnmarshalBlock(nr.GetBytes()); err != nil {
			return nil, err
		}
		return out, nil
	}
	out, ok = blk.(*ExecutionTxData)
	if !ok {
		return out, block.ErrUnexpectedType
	}
	return out, nil
}

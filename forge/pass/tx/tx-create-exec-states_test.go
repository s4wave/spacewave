package pass_tx

import "testing"

// TestNewTxCreateExecSpecsTxnType pins that the constructor returns a
// CREATE_EXEC_SPECS transaction, matching its contract and sibling shapes.
func TestNewTxCreateExecSpecsTxnType(t *testing.T) {
	txn := NewTxCreateExecSpecsTxn()
	if got := txn.GetTxType(); got != TxType_TxType_CREATE_EXEC_SPECS {
		t.Fatalf("expected CREATE_EXEC_SPECS, got %s", got.String())
	}
}

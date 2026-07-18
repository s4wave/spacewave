package execution_tx

import (
	"context"
	"testing"

	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

func TestTxStartAdoptsRunningExecution(t *testing.T) {
	peerID := peer.ID("12D3KooWGVhTGboSk5zPHWcnuw66ysJ29F8r9RYu75qUTxZ83JL8")
	root := &forge_execution.Execution{
		ExecutionState: forge_execution.State_ExecutionState_RUNNING,
	}

	err := NewTxStart(peerID).GetTxStart().ExecuteTx(
		context.Background(), peerID, nil, root,
	)
	if err != nil {
		t.Fatalf("adopt running execution: %v", err)
	}
	if got := root.GetExecutionState(); got != forge_execution.State_ExecutionState_RUNNING {
		t.Fatalf("execution state changed to %s", got)
	}
}

func TestTxCompleteAdoptsCompleteExecution(t *testing.T) {
	root := &forge_execution.Execution{
		ExecutionState: forge_execution.State_ExecutionState_COMPLETE,
		Result:         forge_value.NewResultWithSuccess(),
	}

	err := NewTxComplete(forge_value.NewResultWithSuccess()).GetTxComplete().ExecuteTx(
		context.Background(), peer.ID("12D3KooWGVhTGboSk5zPHWcnuw66ysJ29F8r9RYu75qUTxZ83JL8"), nil, root,
	)
	if err != nil {
		t.Fatalf("adopt complete execution: %v", err)
	}
	if got := root.GetExecutionState(); got != forge_execution.State_ExecutionState_COMPLETE {
		t.Fatalf("execution state changed to %s", got)
	}
}

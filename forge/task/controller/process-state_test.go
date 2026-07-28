package task_controller

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
	forge_task "github.com/s4wave/spacewave/forge/task"
	task_tx "github.com/s4wave/spacewave/forge/task/tx"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestBuildUpdateInputValueSetForTargetOnlyUpdate(t *testing.T) {
	storedInputs := forge_value.ValueSlice{forge_value.NewValue("scheduler-input")}
	valueSet := buildUpdateInputValueSet(storedInputs, nil, nil, nil)

	tx := task_tx.NewTxUpdateInputs("task")
	tx.TxUpdateInputs.UpdateTarget = true
	tx.TxUpdateInputs.ValueSet = valueSet
	if err := tx.Validate(); err != nil {
		t.Fatalf("target-only update with stored inputs is invalid: %v", err)
	}
	if len(valueSet.GetInputs()) != 1 || valueSet.GetInputs()[0].GetName() != "scheduler-input" {
		t.Fatalf("value set inputs = %v, want stored input", valueSet.GetInputs())
	}
}

func TestProcessCheckTaskResultAppliesCompletionWithoutLinkedPassRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ws := &checkTaskWorldState{
		lookupStarted: make(chan struct{}),
		applyCalled:   make(chan world.Operation, 1),
	}
	controller := &Controller{
		le:     logrus.NewEntry(logrus.New()),
		objKey: "task",
		peerID: peer.ID("12D3KooWGVhTGboSk5zPHWcnuw66ysJ29F8r9RYu75qUTxZ83JL8"),
	}
	taskState := &forge_task.Task{PassNonce: 1}
	errCh := make(chan error, 1)
	go func() {
		errCh <- controller.processCheckTaskResult(ctx, ws, taskState)
	}()

	select {
	case op := <-ws.applyCalled:
		tx, ok := op.(*task_tx.Tx)
		if !ok {
			t.Fatalf("completion operation type = %T, want *task_tx.Tx", op)
		}
		if tx.GetTxType() != task_tx.TxType_TxType_COMPLETE {
			t.Fatalf("completion operation type = %s, want COMPLETE", tx.GetTxType())
		}
	case <-time.After(time.Second):
		cancel()
		<-errCh
		t.Fatal("CHECKING processing blocked before ApplyWorldOp")
	}

	if err := <-errCh; err != nil {
		t.Fatalf("processCheckTaskResult: %v", err)
	}
	select {
	case <-ws.lookupStarted:
		t.Fatal("CHECKING processing performed a linked Pass lookup")
	default:
	}
}

type checkTaskWorldState struct {
	world.WorldState
	lookupStarted chan struct{}
	applyCalled   chan world.Operation
}

func (w *checkTaskWorldState) LookupGraphQuads(
	ctx context.Context,
	_ world.GraphQuad,
	_ uint32,
) ([]world.GraphQuad, error) {
	close(w.lookupStarted)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (w *checkTaskWorldState) ApplyWorldOp(
	_ context.Context,
	op world.Operation,
	_ peer.ID,
) (uint64, bool, error) {
	w.applyCalled <- op
	return 0, false, nil
}

package task_tx

import (
	"context"
	"testing"

	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/s4wave/spacewave/net/peer"
)

func TestTxStartAdoptsRunningTask(t *testing.T) {
	root := &forge_task.Task{
		TaskState: forge_task.State_TaskState_RUNNING,
	}

	err := NewTxStart("task", false).GetTxStart().ExecuteTx(
		context.Background(), nil, peer.ID("peer"), "task", nil, root,
	)
	if err != nil {
		t.Fatalf("adopt running task: %v", err)
	}
	if got := root.GetTaskState(); got != forge_task.State_TaskState_RUNNING {
		t.Fatalf("task state changed to %s", got)
	}
}

package forge_task

import (
	"strings"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
)

// WorldTitle returns the name, which object browsers show for the task.
func (e *Task) WorldTitle() string {
	return e.GetName()
}

// WorldState names the task's state for object browsers, and a complete
// task's outcome.
func (e *Task) WorldState() string {
	switch s := e.GetTaskState(); s {
	case State_TaskState_UNKNOWN:
		return ""
	case State_TaskState_COMPLETE:
		return e.GetResult().Outcome()
	default:
		return strings.ToLower(strings.TrimPrefix(s.String(), "TaskState_"))
	}
}

// WorldTime returns when the task was made.
func (e *Task) WorldTime() *timestamppb.Timestamp {
	return e.GetTimestamp()
}

// _ is a type assertion
var (
	_ world.TitledBlock   = (*Task)(nil)
	_ world.StatefulBlock = (*Task)(nil)
	_ world.DatedBlock    = (*Task)(nil)
)

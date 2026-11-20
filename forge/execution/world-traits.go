package forge_execution

import (
	"strings"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
)

// WorldState names the execution's state for object browsers, and a complete
// execution's outcome.
func (e *Execution) WorldState() string {
	switch s := e.GetExecutionState(); s {
	case State_ExecutionState_UNKNOWN:
		return ""
	case State_ExecutionState_COMPLETE:
		return e.GetResult().Outcome()
	default:
		return strings.ToLower(strings.TrimPrefix(s.String(), "ExecutionState_"))
	}
}

// WorldTime returns when the execution was made.
func (e *Execution) WorldTime() *timestamppb.Timestamp {
	return e.GetTimestamp()
}

// _ is a type assertion
var (
	_ world.StatefulBlock = (*Execution)(nil)
	_ world.DatedBlock    = (*Execution)(nil)
)

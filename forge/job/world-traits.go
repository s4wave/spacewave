package forge_job

import (
	"strings"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
)

// WorldState returns the job's state for object browsers, and a complete
// job's outcome.
func (e *Job) WorldState() string {
	switch s := e.GetJobState(); s {
	case State_JobState_UNKNOWN:
		return ""
	case State_JobState_COMPLETE:
		return e.GetResult().Outcome()
	default:
		return strings.ToLower(strings.TrimPrefix(s.String(), "JobState_"))
	}
}

// WorldTime returns when the job was made.
func (e *Job) WorldTime() *timestamppb.Timestamp {
	return e.GetTimestamp()
}

// _ is a type assertion
var (
	_ world.StatefulBlock = (*Job)(nil)
	_ world.DatedBlock    = (*Job)(nil)
)

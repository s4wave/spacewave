package forge_pass

import (
	"strings"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
)

// WorldState names the pass's state for object browsers, and a complete
// pass's outcome.
func (e *Pass) WorldState() string {
	switch s := e.GetPassState(); s {
	case State_PassState_UNKNOWN:
		return ""
	case State_PassState_COMPLETE:
		return e.GetResult().Outcome()
	default:
		return strings.ToLower(strings.TrimPrefix(s.String(), "PassState_"))
	}
}

// WorldTime returns when the pass was made.
func (e *Pass) WorldTime() *timestamppb.Timestamp {
	return e.GetTimestamp()
}

// _ is a type assertion
var (
	_ world.StatefulBlock = (*Pass)(nil)
	_ world.DatedBlock    = (*Pass)(nil)
)

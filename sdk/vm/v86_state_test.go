package s4wave_vm

import (
	"testing"
)

// TestIsValidV86StateTransition covers the full transition matrix for
// SetV86StateOp. Same-state transitions are always rejected. Any -> ERROR
// is always allowed. ERROR -> STOPPED clears the error; other transitions
// out of ERROR are rejected.
func TestIsValidV86StateTransition(t *testing.T) {
	type row struct {
		src   VmState
		dst   VmState
		valid bool
	}

	cases := []row{
		// Valid forward transitions.
		{VmState_VmState_STOPPED, VmState_VmState_STARTING, true},
		{VmState_VmState_STARTING, VmState_VmState_RUNNING, true},
		{VmState_VmState_STARTING, VmState_VmState_STOPPED, true},
		{VmState_VmState_RUNNING, VmState_VmState_STOPPING, true},
		{VmState_VmState_RUNNING, VmState_VmState_STOPPED, true},
		{VmState_VmState_STOPPING, VmState_VmState_STOPPED, true},

		// any -> ERROR.
		{VmState_VmState_STOPPED, VmState_VmState_ERROR, true},
		{VmState_VmState_STARTING, VmState_VmState_ERROR, true},
		{VmState_VmState_RUNNING, VmState_VmState_ERROR, true},
		{VmState_VmState_STOPPING, VmState_VmState_ERROR, true},

		// ERROR reset.
		{VmState_VmState_ERROR, VmState_VmState_STOPPED, true},
		{VmState_VmState_ERROR, VmState_VmState_STARTING, false},
		{VmState_VmState_ERROR, VmState_VmState_RUNNING, false},
		{VmState_VmState_ERROR, VmState_VmState_STOPPING, false},
		{VmState_VmState_ERROR, VmState_VmState_ERROR, false},

		// Invalid jumps.
		{VmState_VmState_STOPPED, VmState_VmState_RUNNING, false},
		{VmState_VmState_STOPPED, VmState_VmState_STOPPING, false},
		{VmState_VmState_STARTING, VmState_VmState_STOPPING, false},
		{VmState_VmState_RUNNING, VmState_VmState_STARTING, false},
		{VmState_VmState_STOPPING, VmState_VmState_RUNNING, false},
		{VmState_VmState_STOPPING, VmState_VmState_STARTING, false},

		// Self-transitions always rejected.
		{VmState_VmState_STOPPED, VmState_VmState_STOPPED, false},
		{VmState_VmState_STARTING, VmState_VmState_STARTING, false},
		{VmState_VmState_RUNNING, VmState_VmState_RUNNING, false},
		{VmState_VmState_STOPPING, VmState_VmState_STOPPING, false},
	}

	for _, c := range cases {
		got := IsValidV86StateTransition(c.src, c.dst)
		if got != c.valid {
			t.Errorf("IsValidV86StateTransition(%s, %s) = %v, want %v",
				c.src.String(), c.dst.String(), got, c.valid)
		}
	}
}

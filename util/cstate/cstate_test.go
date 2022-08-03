package cstate

import (
	"context"
	"testing"
)

// TestCState tests the CState type.
func TestCState(t *testing.T) {
	ctx := context.Background()
	st := NewCState(0)
	lastState := 0
	go func() {
		_ = st.Execute(ctx, nil)
	}()
	st.AddWatcher(ctx, true, func(ctx context.Context, state int) {
		lastState = state
	})
	st.Apply(ctx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
		v.SetObj(1)
		return true, nil
	})
	if lastState != 1 {
		t.Fail()
	}
}

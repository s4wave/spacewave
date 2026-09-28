package dist_entrypoint

import (
	"context"
	"slices"
	"testing"
)

// TestDistBusReleaseOrdersLeaseBeforeRelaunch covers the real distribution
// bus callback queue used by the daemon handoff after listener drain.
func TestDistBusReleaseOrdersLeaseBeforeRelaunch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	stack := &distReleaseStack{}
	stack.add(cancel)
	bus := &DistBus{rel: stack.release, addRelease: stack.add}
	var order []string
	stack.add(func() { order = append(order, "bus resources") })
	bus.AddRelease(func() {
		if ctx.Err() == nil {
			t.Error("bus context still active at lease release")
		}
		order = append(order, "state lease")
	})
	bus.AddRelease(func() { order = append(order, "relaunch") })
	bus.Release()
	if want := []string{"bus resources", "state lease", "relaunch"}; !slices.Equal(order, want) {
		t.Fatalf("distribution release order = %v, want %v", order, want)
	}
}

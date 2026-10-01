//go:build !js

package resource_world

import (
	"strconv"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestTrackedWorldStateConcurrentTrackingKeepsPublishedSnapshots proves
// concurrent access tracking neither races nor mutates published snapshots.
func TestTrackedWorldStateConcurrentTrackingKeepsPublishedSnapshots(t *testing.T) {

	// context ctx.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	defer tb.Release()

	// newEngineWorldState ws via world.
	ws := world.NewEngineWorldState(tb.Engine, false)
	tracked := NewTrackedWorldState(ws, ws, 0, ctx)
	defer tracked.Close()

	// trackObjectAccess via tracked.
	tracked.trackObjectAccess("obj/shared", 1)
	tracked.mtx.Lock()
	published := tracked.currentSnapshot
	tracked.mtx.Unlock()

	// Perform the action.
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 20 {
				tracked.trackObjectAccess("obj/shared", uint64(2+j))
				tracked.trackObjectAccess("obj/"+strconv.Itoa(i)+"/"+strconv.Itoa(j), 1)
				tracked.trackQuadQuery()
			}
		})
	}
	wg.Wait()

	// Check the condition before continuing.
	if len(published.GetObjectAccesses()) != 1 || published.GetObjectAccesses()[0].GetRev() != 1 {
		t.Fatalf("published snapshot mutated: %v", published)
	}
	tracked.mtx.Lock()
	current := tracked.currentSnapshot
	tracked.mtx.Unlock()
	if got, want := len(current.GetObjectAccesses()), 1+8*20; got != want {
		t.Fatalf("tracked accesses = %d, want %d", got, want)
	}
}

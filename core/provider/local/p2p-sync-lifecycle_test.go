package provider_local

import "testing"

func TestP2PSyncStateStartCompletionIsIdempotent(t *testing.T) {
	state := &p2pSyncState{startDone: make(chan struct{})}

	state.completeStart()
	state.completeStart()

	select {
	case <-state.startDone:
	default:
		t.Fatal("expected startup completion signal to be closed")
	}
}

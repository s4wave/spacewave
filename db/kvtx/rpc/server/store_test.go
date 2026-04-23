package kvtx_rpc_server

import (
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
)

func TestTxHandleCloseOpsWaitsForActiveStreams(t *testing.T) {
	h := &txHandle{
		active: make(map[uint64]func()),
		idle:   make(chan struct{}),
	}

	released := make(chan struct{})
	_, release, err := h.acquire(func() {
		close(released)
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	closed := make(chan struct{})
	go func() {
		h.closeOps()
		close(closed)
	}()

	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("closeOps did not release active stream")
	}

	select {
	case <-closed:
		t.Fatal("closeOps returned before active stream released")
	default:
	}

	release()

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("closeOps did not return after active stream released")
	}

	_, _, err = h.acquire(nil)
	if !errors.Is(err, kvtx.ErrDiscarded) {
		t.Fatalf("acquire after close error = %v, want %v", err, kvtx.ErrDiscarded)
	}
}

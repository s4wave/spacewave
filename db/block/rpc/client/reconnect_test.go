package block_rpc_client

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
)

// failingReads answers each read with the next error of errs, then found.
type failingReads struct {
	block.StoreOps
	errs  []error
	calls int
}

// GetBlock returns the next error, or a found block once errs run out.
func (s *failingReads) GetBlock(context.Context, *block.BlockRef) ([]byte, bool, error) {
	s.calls++
	if len(s.errs) == 0 {
		return []byte("ok"), true, nil
	}
	err := s.errs[0]
	s.errs = s.errs[1:]
	return nil, false, err
}

// TestReconnectReads retries reads that lost their connection, returns a
// store's own error at once, and stops retrying when ctx ends.
func TestReconnectReads(t *testing.T) {
	// Lost connections are sent again until the read answers.
	lost := &failingReads{errs: []error{srpc.ErrClosedBeforeCompletion, srpc.ErrReset, srpc.ErrNoAvailableClients}}
	if _, found, err := ReconnectReads(lost).GetBlock(t.Context(), nil); err != nil || !found || lost.calls != 4 {
		t.Fatalf("found %v after %d calls: %v", found, lost.calls, err)
	}

	// A store error is the answer.
	refused := &failingReads{errs: []error{errors.New("store refused")}}
	if _, _, err := ReconnectReads(refused).GetBlock(t.Context(), nil); err == nil || refused.calls != 1 {
		t.Fatalf("retried a store error %d times: %v", refused.calls, err)
	}

	// A connection that never returns ends with ctx.
	down := &failingReads{errs: make([]error, 1000)}
	for i := range down.errs {
		down.errs[i] = srpc.ErrReset
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if _, _, err := ReconnectReads(down).GetBlock(ctx, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("read past its deadline: %v", err)
	}
}

package provider_spacewave

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
)

// TestBV3ProbeRecheckLostOnPushError: a write of a held hash during a flush
// sets the recheck; the next flush consumes it and fails on a push error
// before setCorruptKnown; does the flush after that read the block?
func TestBV3ProbeRecheckLostOnPushError(t *testing.T) {
	var uploads, failed atomic.Int32
	var failing atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/push") {
			if failing.Load() {
				failed.Add(1)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			uploads.Add(1)
		}
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, server.URL, DefaultSigningEnvPrefix, priv, pid.String())

	s := newDirtySyncExecuteTestController(t, cli, nil)
	good := s.upper
	ref, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.upper = &corruptUpperStore{StoreOps: good, bad: ref.GetHash()}
	store := &pausingReadStore{Store: s.store, paused: make(chan struct{}), release: make(chan struct{})}
	s.store = store
	var held *heldError
	if err := s.FlushNow(t.Context()); !errors.As(err, &held) {
		t.Fatalf("first flush = %v; want held", err)
	}

	// Pass N: paused after its known copy; repair and write the block.
	store.pauseAt.Store(store.reads.Load() + 2)
	flushed := make(chan error, 1)
	go func() { flushed <- s.FlushNow(t.Context()) }()
	select {
	case <-store.paused:
	case err := <-flushed:
		t.Fatalf("flush ended before its pause: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("flush never reached its pause")
	}
	if err := good.RmBlock(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if _, _, err := good.PutBlock(t.Context(), []byte("dirty block"), nil); err != nil {
		t.Fatal(err)
	}
	s.upper = good
	if err := s.MarkDirty(t.Context(), []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len("dirty block"))}}); err != nil {
		t.Fatal(err)
	}
	close(store.release)
	errN := <-flushed
	t.Logf("BV3PROBE pass N err=%v recheck=%v known=%v", errN, s.corruptRecheck.Load(), hasKey(s.corruptKnown, ref.GetHash().MarshalString()))

	// Pass N+1: the push fails.
	failing.Store(true)
	errN1 := s.FlushNow(t.Context())
	failing.Store(false)
	t.Logf("BV3PROBE pass N+1 err=%v failed pushes=%d recheck=%v known=%v", errN1, failed.Load(), s.corruptRecheck.Load(), hasKey(s.corruptKnown, ref.GetHash().MarshalString()))

	// Pass N+2: no write, no deadline.
	errN2 := s.FlushNow(t.Context())
	t.Logf("BV3PROBE pass N+2 err=%v uploads=%d known=%v", errN2, uploads.Load(), hasKey(s.corruptKnown, ref.GetHash().MarshalString()))
	if errors.As(errN2, &held) {
		t.Fatalf("BV3PROBE: sound rewritten block held without a read after a failed push consumed the recheck")
	}
}

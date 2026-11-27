package provider_spacewave

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx"
)

// bv2Store counts read transactions and can pause one.
type bv2Store struct {
	kvtx.Store
	mu      sync.Mutex
	times   []time.Duration
	start   time.Time
	pauseAt atomic.Int64 // pause the read tx with this ordinal (1-based) after arming
	reads   atomic.Int64
	paused  chan struct{}
	release chan struct{}
}

func (s *bv2Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write {
		n := s.reads.Add(1)
		s.mu.Lock()
		s.times = append(s.times, time.Since(s.start).Round(100*time.Millisecond))
		s.mu.Unlock()
		if p := s.pauseAt.Load(); p != 0 && n == p {
			close(s.paused)
			<-s.release
		}
	}
	return s.Store.NewTransaction(ctx, write)
}

// countingCorrupt counts reads of the bad hash.
type countingCorrupt struct {
	corruptUpperStore
	badReads atomic.Int64
}

func (s *countingCorrupt) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if ref.GetHash().EqualVT(s.bad) {
		s.badReads.Add(1)
	}
	return s.corruptUpperStore.GetStoredBlock(ctx, ref)
}

func TestBv2ProbeExecuteIdle(t *testing.T) {
	s := newDirtySyncExecuteTestController(t, nil, nil)
	ref, _ := block.BuildBlockRef([]byte("dirty block"), nil)
	cs := &countingCorrupt{corruptUpperStore: corruptUpperStore{StoreOps: s.upper, bad: ref.GetHash()}}
	s.upper = cs
	st := &bv2Store{Store: s.store, start: time.Now()}
	s.store = st
	s.conf = &SyncConfig{SizeThresholdBytes: 1 << 30, CheckpointIntervalSecs: 1}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Execute(ctx) }()
	time.Sleep(6500 * time.Millisecond)
	st.mu.Lock()
	times := append([]time.Duration(nil), st.times...)
	st.mu.Unlock()
	fmt.Printf("BV2PROBE idle: over 6.5s read txs=%d bad reads=%d at %v\n", len(times), cs.badReads.Load(), times)
	cancel()
	<-done
}

func TestBv2ProbeRepairDuringFlush(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	s := newDirtySyncExecuteTestController(t, cli, nil)
	good := s.upper
	ref, _ := block.BuildBlockRef([]byte("dirty block"), nil)
	s.upper = &corruptUpperStore{StoreOps: good, bad: ref.GetHash()}
	st := &bv2Store{Store: s.store, start: time.Now(), paused: make(chan struct{}), release: make(chan struct{})}
	s.store = st
	ctx := context.Background()

	// First flush holds the block and records it known.
	err := s.FlushNow(ctx)
	fmt.Printf("BV2PROBE race: first flush err=%v known=%v\n", err, hasKey(s.corruptKnown, ref.GetHash().MarshalString()))

	// Second flush: pause at its second read tx (after the known snapshot),
	// repair the bytes and write the hash, then let the flush finish.
	base := st.reads.Load()
	st.pauseAt.Store(base + 2)
	flushed := make(chan error, 1)
	go func() { flushed <- s.FlushNow(ctx) }()
	select {
	case <-st.paused:
	case err := <-flushed:
		t.Fatalf("flush ended before pause: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("no pause")
	}
	s.upper = good
	if err := s.MarkDirty(ctx, []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len("dirty block"))}}); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("BV2PROBE race: after MarkDirty mid-flush known=%v\n", hasKey(s.corruptKnown, ref.GetHash().MarshalString()))
	close(st.release)
	err = <-flushed
	fmt.Printf("BV2PROBE race: mid-flush pass err=%v; known after it=%v\n", err, hasKey(s.corruptKnown, ref.GetHash().MarshalString()))
	st.pauseAt.Store(0)

	// Third flush with repaired bytes and no recheck: does it read the block?
	err = s.FlushNow(ctx)
	fmt.Printf("BV2PROBE race: flush after repair err=%v\n", err)
}

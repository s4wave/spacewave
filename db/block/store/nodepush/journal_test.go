package block_store_nodepush

import (
	"context"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_journal "github.com/s4wave/spacewave/db/kvtx/journal"
	"github.com/sirupsen/logrus"
)

// crashStore is an in-memory store with ordered commits whose crash keeps
// what its last flush covered and any prefix of the commits after it, as a
// write-ahead log does. Its flush can be held, as a stalled disk holds it.
type crashStore struct {
	turn sync.Mutex

	mtx sync.Mutex
	// durable is the state the last flush covered.
	durable map[string][]byte
	// pending are the commits since, in order.
	pending []map[string]*[]byte
	// held makes Sync wait until released.
	held chan struct{}
}

func newCrashStore(durable map[string][]byte) *crashStore {
	return &crashStore{durable: durable}
}

// view returns the state with every commit applied.
func (c *crashStore) view() map[string][]byte {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	out := maps.Clone(c.durable)
	for _, commit := range c.pending {
		apply(out, commit)
	}
	return out
}

func apply(state map[string][]byte, commit map[string]*[]byte) {
	for k, v := range commit {
		if v == nil {
			delete(state, k)
		} else {
			state[k] = *v
		}
	}
}

// crash returns the store a host crash leaves: the flushed state and the
// first keep commits after it.
func (c *crashStore) crash(keep int) *crashStore {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	out := maps.Clone(c.durable)
	for _, commit := range c.pending[:min(keep, len(c.pending))] {
		apply(out, commit)
	}
	return newCrashStore(out)
}

func (c *crashStore) hold() {
	c.mtx.Lock()
	c.held = make(chan struct{})
	c.mtx.Unlock()
}

func (c *crashStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write {
		c.turn.Lock()
	}
	return &crashTx{c: c, base: c.view(), writes: make(map[string]*[]byte), write: write}, nil
}

func (c *crashStore) Sync(ctx context.Context) error {
	c.mtx.Lock()
	held := c.held
	c.mtx.Unlock()
	if held != nil {
		select {
		case <-held:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	for _, commit := range c.pending {
		apply(c.durable, commit)
	}
	c.pending = nil
	return nil
}

func (c *crashStore) WaitDurable(ctx context.Context) error { return c.Sync(ctx) }

type crashTx struct {
	c      *crashStore
	base   map[string][]byte
	writes map[string]*[]byte
	write  bool
	done   bool
}

func (t *crashTx) get(key []byte) ([]byte, bool) {
	if v, ok := t.writes[string(key)]; ok {
		if v == nil {
			return nil, false
		}
		return *v, true
	}
	v, ok := t.base[string(key)]
	return v, ok
}

func (t *crashTx) Size(context.Context) (uint64, error) { return uint64(len(t.base)), nil }

func (t *crashTx) Get(_ context.Context, key []byte) ([]byte, bool, error) {
	v, ok := t.get(key)
	return v, ok, nil
}

func (t *crashTx) Exists(_ context.Context, key []byte) (bool, error) {
	_, ok := t.get(key)
	return ok, nil
}

func (t *crashTx) Set(_ context.Context, key, value []byte) error {
	v := slices.Clone(value)
	t.writes[string(key)] = &v
	return nil
}

func (t *crashTx) Delete(_ context.Context, key []byte) error {
	t.writes[string(key)] = nil
	return nil
}

func (t *crashTx) ScanPrefix(context.Context, []byte, func(key, value []byte) error) error {
	return kvtx.ErrNotFound
}

func (t *crashTx) ScanPrefixKeys(context.Context, []byte, func(key []byte) error) error {
	return kvtx.ErrNotFound
}

func (t *crashTx) Iterate(context.Context, []byte, bool, bool) kvtx.Iterator {
	return kvtx.NewErrIterator(kvtx.ErrNotFound)
}

func (t *crashTx) CommitOrdered(context.Context) error {
	if t.done {
		return kvtx.ErrDiscarded
	}
	t.done = true
	if t.write {
		t.c.mtx.Lock()
		t.c.pending = append(t.c.pending, t.writes)
		t.c.mtx.Unlock()
		t.c.turn.Unlock()
	}
	return nil
}

func (t *crashTx) Commit(ctx context.Context) error {
	if err := t.CommitOrdered(ctx); err != nil {
		return err
	}
	return t.c.Sync(ctx)
}

func (t *crashTx) Discard() {
	if !t.done {
		t.done = true
		if t.write {
			t.c.turn.Unlock()
		}
	}
}

// journalNode is a storage node serving its journal from dir.
type journalNode struct {
	journal *Journal
	server  *Server
	client  *Client
	stop    func()
}

func startNode(ctx context.Context, t *testing.T, dir string, up bool) *journalNode {
	t.Helper()
	journal, err := OpenJournal(filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(nopHolder{}, filepath.Join(dir, "reports"))
	if err != nil {
		t.Fatal(err)
	}
	server.SetJournal(journal)
	n := &journalNode{journal: journal, server: server, client: NewClient(logrus.NewEntry(logrus.New()), nil), stop: func() {}}
	if up {
		n.stop = connect(ctx, t, n.client, server)
	} else {
		n.stop = func() {}
	}
	return n
}

func (n *journalNode) close() {
	n.stop()
	_ = n.journal.Close()
}

// TestJournalReplaysAfterCrash crashes the local store after the node
// flushed its commits' records and before its own flush, at every possible
// point of its write-ahead log, restarts the node too, and checks that the
// replay brings back every acknowledged write, then that a restart which
// cannot reach the node starts on its local head and the node sets that run
// aside once it is back.
func TestJournalReplaysAfterCrash(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	rng := rand.New(rand.NewPCG(1, 2))
	live := func() bool { return true }
	opts := kvtx_journal.Options{Live: live, Wait: 30 * time.Second, StartWait: 5 * time.Second, FlushEvery: 5 * time.Millisecond}

	// Go live on a fresh store, then stall its flush.
	n := startNode(ctx, t, dir, true)
	inner := newCrashStore(make(map[string][]byte))
	store, err := kvtx_journal.Open(ctx, inner, JournalSink{Client: n.client}, opts)
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stopRun := context.WithCancel(ctx)
	go func() { _ = store.Run(runCtx) }()
	waitFor(t, func() bool {
		inner.mtx.Lock()
		defer inner.mtx.Unlock()
		head := &kvtx_journal.Head{}
		return head.UnmarshalVT(inner.durable[string(kvtx_journal.HeadKey)]) == nil && head.GetLive()
	})
	inner.hold()

	// Commit sets and deletes; each acknowledged write is expected back.
	acked := make(map[string]string)
	for i := range 60 {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		key := "k" + strconv.Itoa(rng.IntN(12))
		var value string
		if rng.IntN(4) == 0 {
			err = tx.Delete(ctx, []byte(key))
		} else {
			value = "v" + strconv.Itoa(i)
			err = tx.Set(ctx, []byte(key), []byte(value))
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		acked[key] = value
	}

	// Nothing reached the local flush.
	inner.mtx.Lock()
	pending := len(inner.pending)
	inner.mtx.Unlock()
	if pending < 60 {
		t.Fatalf("%d commits pending the local flush, want every commit since going live", pending)
	}
	stopRun()

	// Crash at each point of the write-ahead log, with a node restart, and
	// replay. The node keeps its journal across the cases: only the last
	// replay cuts it.
	check := func(store *kvtx_journal.Store) {
		t.Helper()
		tx, err := store.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		for key, want := range acked {
			got, found, err := tx.Get(ctx, []byte(key))
			if err != nil {
				t.Fatal(err)
			}
			if (want == "") == found || string(got) != want {
				t.Fatalf("%s is %q (found %v) after replay, want %q", key, got, found, want)
			}
		}
	}
	n.close()
	for _, keep := range []int{0, 1, pending / 2, pending - 1} {
		crashed := inner.crash(keep)
		journalDir := filepath.Join(dir, "journal")
		copyDir := filepath.Join(t.TempDir(), "node")
		if err := os.MkdirAll(filepath.Join(copyDir, "journal"), 0o700); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(journalDir, journalFile))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(copyDir, "journal", journalFile), data, 0o600); err != nil {
			t.Fatal(err)
		}
		n := startNode(ctx, t, copyDir, true)
		replayed, err := kvtx_journal.Open(ctx, crashed, JournalSink{Client: n.client}, opts)
		if err != nil {
			t.Fatal(err)
		}
		check(replayed)
		n.close()
	}

	// The real node: replay, then the old run is cut and refused.
	n = startNode(ctx, t, dir, true)
	crashed := inner.crash(0)
	replayed, err := kvtx_journal.Open(ctx, crashed, JournalSink{Client: n.client}, opts)
	if err != nil {
		t.Fatal(err)
	}
	check(replayed)
	var oldEpoch uint64
	for e := range n.journal.cut {
		oldEpoch = e
	}
	if oldEpoch == 0 || len(n.journal.records) != 0 {
		t.Fatalf("journal holds %d runs and cut %d after replay, want none and the old run", len(n.journal.records), oldEpoch)
	}
	if err := n.journal.Append(&Record{Epoch: oldEpoch, Seq: 1000}); err == nil {
		t.Fatal("a record of the cut run was taken")
	}

	// Go live again, stall, commit, crash, and restart with the node away:
	// the store starts on its local head, and the node sets the run aside
	// once it is back.
	runCtx, stopRun = context.WithCancel(ctx)
	go func() { _ = replayed.Run(runCtx) }()
	waitFor(t, func() bool {
		crashed.mtx.Lock()
		defer crashed.mtx.Unlock()
		head := &kvtx_journal.Head{}
		return head.UnmarshalVT(crashed.durable[string(kvtx_journal.HeadKey)]) == nil && head.GetLive()
	})
	crashed.hold()
	for i := range 5 {
		tx, _ := replayed.NewTransaction(ctx, true)
		_ = tx.Set(ctx, []byte("late"), []byte(strconv.Itoa(i)))
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	stopRun()
	n.close()
	away := startNode(ctx, t, dir, false)
	short := opts
	short.StartWait = 50 * time.Millisecond
	alone, err := kvtx_journal.Open(ctx, crashed.crash(0), JournalSink{Client: away.client}, short)
	if err != nil {
		t.Fatal(err)
	}
	away.stop = connect(ctx, t, away.client, away.server)
	defer away.close()
	runCtx, stopRun = context.WithCancel(ctx)
	defer stopRun()
	go func() { _ = alone.Run(runCtx) }()
	waitFor(t, func() bool {
		names, _ := os.ReadDir(filepath.Join(dir, "journal"))
		return slices.ContainsFunc(names, func(e os.DirEntry) bool { return strings.HasPrefix(e.Name(), "quarantine-") })
	})
}

// TestJournalOffWaitsForLocalFlush checks that with the marker off, which is
// the "on" mode and every start before going live, WaitDurable waits for the
// inner store's flush rather than returning at once.
func TestJournalOffWaitsForLocalFlush(t *testing.T) {
	ctx := context.Background()
	inner := newCrashStore(make(map[string][]byte))
	store, err := kvtx_journal.Open(ctx, inner, JournalSink{Client: NewClient(logrus.NewEntry(logrus.New()), nil)}, kvtx_journal.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// Commit ordered with the flush stalled.
	inner.hold()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Set(ctx, []byte("k"), []byte("v"))
	if err := kvtx.CommitOrdered(ctx, tx); err != nil {
		t.Fatal(err)
	}

	// WaitDurable waits for the flush.
	done := make(chan error, 1)
	go func() { done <- store.WaitDurable(ctx) }()
	select {
	case <-done:
		t.Fatal("WaitDurable returned with the commit not flushed")
	case <-time.After(50 * time.Millisecond):
	}
	inner.mtx.Lock()
	close(inner.held)
	inner.held = nil
	inner.mtx.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

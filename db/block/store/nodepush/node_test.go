package block_store_nodepush

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/sirupsen/logrus"
)

// memRemote is the SFTP server: it holds uploaded packs.
type memRemote struct {
	mtx   sync.Mutex
	packs map[string][]byte
	puts  int
}

func (m *memRemote) HasPackfile(_ context.Context, id string) (bool, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	_, ok := m.packs[id]
	return ok, nil
}

func (m *memRemote) PutPackFile(_ context.Context, id string, data []byte) (*packfile.PackfileEntry, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.packs[id] = data
	m.puts++
	return &packfile.PackfileEntry{Id: id, SizeBytes: uint64(len(data))}, nil
}

// applier records the reports it took, failing while refuse is set.
type applier struct {
	mtx     sync.Mutex
	refuse  bool
	applied map[string][][]byte
	changed chan struct{}
}

func (a *applier) ApplyPack(_ context.Context, report *PackReport) error {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if a.refuse {
		select {
		case a.changed <- struct{}{}:
		default:
		}
		return errors.New("index busy")
	}
	a.applied[report.GetPackId()] = report.GetKeys()
	select {
	case a.changed <- struct{}{}:
	default:
	}
	return nil
}

func (a *applier) count() int {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return len(a.applied)
}

func entry(t *testing.T, data string) *block.PutBatchEntry {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	return &block.PutBatchEntry{Ref: ref, Data: []byte(data)}
}

// connect serves one in-memory connection and returns the function ending it.
func connect(ctx context.Context, t *testing.T, c *Client, s *Server) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	near, far := net.Pipe()
	var done sync.WaitGroup
	done.Add(2)
	go func() { defer done.Done(); _ = c.serve(ctx, near) }()
	go func() { defer done.Done(); _ = s.Serve(ctx, far) }()
	waitFor(t, func() bool {
		c.mtx.Lock()
		defer c.mtx.Unlock()
		return c.out != nil
	})
	return func() {
		cancel()
		_ = near.Close()
		_ = far.Close()
		done.Wait()
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestNodeRunThrough walks blocks through a real pack log: a flushed push, a
// corrupted push, an upload whose report outlives a refused apply and a lost
// connection, a node restart, and a node that is away.
func TestNodeRunThrough(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	log, err := packahead.Open(ctx, dir+"/log", packahead.Options{Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	server, err := NewServer(log, dir+"/reports")
	if err != nil {
		t.Fatal(err)
	}
	apply := &applier{refuse: true, applied: map[string][][]byte{}, changed: make(chan struct{}, 1)}
	client := NewClient(logrus.NewEntry(logrus.New()), apply)

	// A node that is away answers at once, so a commit never waits on it.
	if err := client.Send(ctx, []*block.PutBatchEntry{entry(t, "away")}); !errors.Is(err, ErrNodeAway) {
		t.Fatalf("send to an absent node: %v", err)
	}

	// A flushed push is durable in the log when Send returns.
	end := connect(ctx, t, client, server)
	first := entry(t, "first block")
	if err := client.Send(ctx, []*block.PutBatchEntry{first}); err != nil {
		t.Fatal(err)
	}
	key := packfile.BlockKey(first.Ref.GetHash())
	if !log.Has(key) {
		t.Fatal("flushed block not in the log")
	}
	value, err := block.EncodeBlockObject(first.Data, first.Refs, first.RefsKnown)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok, err := log.Get(key); err != nil || !ok || string(got) != string(value) {
		t.Fatalf("log holds %q ok %v err %v", got, ok, err)
	}

	// A corrupted push is refused and leaves nothing behind.
	bad := &Block{Key: packfile.BlockKey(entry(t, "bad").Ref.GetHash()), Data: []byte("bad")}
	if got := server.hold(ctx, &Push{Id: 9, Blocks: []*Block{bad}, Sum: []byte("wrong")}); got.GetRefused() == nil {
		t.Fatalf("corrupted push answered %v", got)
	}
	if log.Has(bad.GetKey()) {
		t.Fatal("corrupted push reached the log")
	}

	// The drain uploads the pack; the first apply fails.
	remote := &memRemote{packs: map[string][]byte{}}
	reporting := NewReportingRemote(remote, server)
	drainCtx, stopDrain := context.WithCancel(ctx)
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		_ = packahead.NewDrain(log, packahead.DrainOptions{}).Run(drainCtx, reporting)
	}()
	<-apply.changed
	if apply.count() != 0 || remote.puts != 1 {
		t.Fatalf("applied %d, puts %d", apply.count(), remote.puts)
	}

	// The connection drops before anyone applied the report. The next
	// connection hears it again, and the apply goes through.
	end()
	apply.mtx.Lock()
	apply.refuse = false
	apply.mtx.Unlock()
	end = connect(ctx, t, client, server)
	waitFor(t, func() bool { return apply.count() == 1 })
	for id, keys := range apply.applied {
		if len(keys) != 1 || string(keys[0]) != string(key) {
			t.Fatalf("pack %s reported keys %x", id, keys)
		}
		waitFor(t, func() bool {
			data, err := os.ReadFile(server.reportPath(id))
			return err == nil && len(data) == 0
		})
	}

	// An applied pack is not reported again, and its upload stands.
	end()
	apply.mtx.Lock()
	apply.applied = map[string][][]byte{}
	apply.mtx.Unlock()
	end = connect(ctx, t, client, server)
	time.Sleep(20 * time.Millisecond)
	if apply.count() != 0 {
		t.Fatal("an applied pack was reported again")
	}
	stopDrain()
	<-drained

	// A crash between the upload and the report leaves no report, so a
	// restarted drain uploads the pack again instead of skipping it.
	remote.packs["ghost"] = []byte("uploaded, never reported")
	if has, err := reporting.HasPackfile(ctx, "ghost"); err != nil || has {
		t.Fatalf("unreported pack counted as uploaded: %v %v", has, err)
	}

	// A connection dropping just after an answer must not wedge later sends:
	// the answer is still undelivered when the second one arrives.
	answered := make(chan error, 1)
	client.mtx.Lock()
	client.waiting[99] = answered
	client.mtx.Unlock()
	client.answer(99, nil)
	client.answer(99, ErrNodeAway)
	client.forget(99)

	// The node going away fails a push in flight at once.
	end()
	if err := client.Send(ctx, []*block.PutBatchEntry{entry(t, "late")}); !errors.Is(err, ErrNodeAway) {
		t.Fatalf("send after the connection ended: %v", err)
	}
}

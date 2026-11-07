package block_store_nodepush

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
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
	deadline := time.Now().Add(60 * time.Second)
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
	ctx := t.Context()
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
	server.wait, server.retryBase = 20*time.Millisecond, time.Millisecond
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
	waitFor(t, func() bool { return nextPending(server) == "" })
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

// nopHolder takes every push at once.
type nopHolder struct{}

// Append holds nothing.
func (nopHolder) Append(context.Context, []packahead.Block) error { return nil }

// slowApplier takes delay to apply each report, and refuses the pack stuck.
type slowApplier struct {
	delay   atomic.Int64
	applied atomic.Int64
	mtx     sync.Mutex
	stuck   string
}

// ApplyPack counts the report after the delay.
func (a *slowApplier) ApplyPack(ctx context.Context, report *PackReport) error {
	a.mtx.Lock()
	stuck := a.stuck
	a.mtx.Unlock()
	if report.GetPackId() == stuck {
		return errors.New("pack merged away")
	}
	select {
	case <-time.After(time.Duration(a.delay.Load())):
	case <-ctx.Done():
		return ctx.Err()
	}
	a.applied.Add(1)
	return nil
}

// TestReportsNeverWedgeCommits runs a backlog of large reports beside commit
// load over a real socket, the load that once filled both directions and
// stopped the connection for good. One report that can never apply must not
// hold back the rest, and every other must apply on a live connection.
func TestReportsNeverWedgeCommits(t *testing.T) {
	// A node with a backlog, starting with a report that never applies.
	ctx := t.Context()
	dir := t.TempDir()
	keys := make([][]byte, 2000)
	for i := range keys {
		keys[i] = make([]byte, 34)
	}
	const packs = 300
	for i := range packs {
		id := fmt.Sprintf("pack%04d", i)
		data, err := (&PackReport{PackId: id, Keys: keys}).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if err := writeDurable(filepath.Join(dir, id+reportSuffix), data); err != nil {
			t.Fatal(err)
		}
	}
	server, err := NewServer(nopHolder{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	server.wait, server.retryBase = 30*time.Millisecond, time.Millisecond

	// Connect over TCP.
	apply := &slowApplier{stuck: "pack0000"}
	apply.delay.Store(int64(time.Millisecond))
	client := NewClient(logrus.NewEntry(logrus.New()), apply)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		if conn, err := listener.Accept(); err == nil {
			_ = server.Serve(ctx, conn)
		}
	}()
	go func() {
		_ = client.Run(ctx, func(context.Context) (net.Conn, error) {
			return net.Dial("tcp", listener.Addr().String())
		}, time.Millisecond)
	}()
	waitFor(t, func() bool {
		client.mtx.Lock()
		defer client.mtx.Unlock()
		return client.out != nil
	})

	// Commit load throughout, giving up quickly as a Publisher does.
	big := entry(t, strings.Repeat("x", 1<<20))
	stop := make(chan struct{})
	var load sync.WaitGroup
	for range 4 {
		load.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				sendCtx, done := context.WithTimeout(ctx, 20*time.Millisecond)
				_ = client.Send(sendCtx, []*block.PutBatchEntry{big})
				done()
			}
		})
	}

	// Every report but the stuck one applies, with the load still running.
	waitFor(t, func() bool { return apply.applied.Load() == packs-1 })
	close(stop)
	load.Wait()

	// The connection still answers a commit at once.
	sendCtx, done := context.WithTimeout(ctx, 3*time.Second)
	defer done()
	if err := client.Send(sendCtx, []*block.PutBatchEntry{entry(t, "after")}); err != nil {
		t.Fatalf("send after the load: %v", err)
	}
}

// TestReportWaitsOnNoConnection checks that an upload finishes with no
// connection up, and that the report reaches the next one.
func TestReportWaitsOnNoConnection(t *testing.T) {
	// Report more packs than any queue holds, with nobody connected.
	ctx := t.Context()
	server, err := NewServer(nopHolder{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 * maxFramesQueued {
		if err := server.report(&PackReport{PackId: fmt.Sprintf("pack%04d", i), Keys: [][]byte{{1}}}); err != nil {
			t.Fatal(err)
		}
	}

	// A connection hears them all.
	apply := &slowApplier{}
	client := NewClient(logrus.NewEntry(logrus.New()), apply)
	defer connect(ctx, t, client, server)()
	waitFor(t, func() bool { return apply.applied.Load() == 3*maxFramesQueued })
}

// objectRemote is the remote's object store.
type objectRemote struct {
	objects map[string][]byte
}

// GetObject opens an object.
func (o *objectRemote) GetObject(_ context.Context, _, key string) (io.ReadCloser, error) {
	data, ok := o.objects[key]
	if !ok {
		return nil, errors.New("no such object")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// TestReadThrough reads ranges through the node: a miss fills the pack, the
// next read comes from the node, and a short range, a bad name, the switch and
// a node that is away or silent all report false rather than an answer.
func TestReadThrough(t *testing.T) {
	// A node with no packs, and a remote with one.
	ctx := t.Context()
	id := identity.NewPackID()
	remote := &objectRemote{objects: map[string][]byte{"p/packs/" + id: []byte("the pack's bytes")}}
	dir := t.TempDir()
	packs, err := NewPackFiles(logrus.NewEntry(logrus.New()), dir+"/packs", remote, "bucket", "p/packs")
	if err != nil {
		t.Fatal(err)
	}
	// The fill routine ends before the directory goes: the context ends first.
	running := make(chan struct{})
	go func() {
		defer close(running)
		packs.Run(ctx)
	}()
	t.Cleanup(func() { <-running })
	server, err := NewServer(nopHolder{}, dir+"/reports")
	if err != nil {
		t.Fatal(err)
	}
	server.SetPacks(packs)
	client := NewClient(logrus.NewEntry(logrus.New()), &slowApplier{})
	end := connect(ctx, t, client, server)
	var on atomic.Bool
	var reach atomic.Pointer[Client]
	reads := NewReads(time.Second, &on, &reach)
	reach.Store(client)
	on.Store(true)

	// A miss answers false and starts the fill.
	if _, ok := reads.ReadCached(ctx, id, 4, 4); ok {
		t.Fatal("read a pack the node lacks")
	}
	waitFor(t, func() bool {
		_, ok := packs.ReadRange(id, 0, 1)
		return ok
	})

	// The fill serves the next read.
	if data, ok := reads.ReadCached(ctx, id, 4, 4); !ok || string(data) != "pack" {
		t.Fatalf("hit read %q %v", data, ok)
	}

	// A range past the end is no answer: a copy shorter than the remote's must
	// never pass for a whole extent.
	if data, ok := reads.ReadCached(ctx, id, 12, 100); ok {
		t.Fatalf("short range answered %q", data)
	}

	// A name that leaves the directory reaches no file.
	if _, ok := client.ReadRange(ctx, "../reports/x", 0, 4); ok {
		t.Fatal("read outside the pack directory")
	}

	// The switch keeps reads off the node.
	on.Store(false)
	if _, ok := reads.ReadCached(ctx, id, 4, 4); ok {
		t.Fatal("read with the switch off")
	}
	on.Store(true)

	// A full disk leaves the pack unfilled.
	full := identity.NewPackID()
	remote.objects["p/packs/"+full] = []byte("no room")
	packs.SetRoom(func() bool { return false })
	if err := packs.Fill(ctx, full); err != nil {
		t.Fatal(err)
	}
	if _, ok := packs.ReadRange(full, 0, 1); ok {
		t.Fatal("filled a pack with no room")
	}
	packs.SetRoom(nil)

	// A node that goes away answers false at once.
	end()
	if _, ok := reads.ReadCached(ctx, id, 4, 4); ok {
		t.Fatal("read from a node that went away")
	}
}

// TestHungReadsHoldNoPush fills every read slot with reads that never finish,
// and a push behind them must still answer.
func TestHungReadsHoldNoPush(t *testing.T) {
	// A pack that is a FIFO hangs its reader, as a hung disk does.
	ctx := t.Context()
	dir := t.TempDir()
	packs, err := NewPackFiles(logrus.NewEntry(logrus.New()), dir+"/packs", &objectRemote{}, "bucket", "p/packs")
	if err != nil {
		t.Fatal(err)
	}
	id := identity.NewPackID()
	if err := syscall.Mkfifo(filepath.Join(dir, "packs", id), 0o600); err != nil {
		t.Skip("no fifo here")
	}
	server, err := NewServer(nopHolder{}, dir+"/reports")
	if err != nil {
		t.Fatal(err)
	}
	server.SetPacks(packs)
	client := NewClient(logrus.NewEntry(logrus.New()), &slowApplier{})
	end := connect(ctx, t, client, server)

	// More reads than slots, none of which will ever answer.
	hung, stopHung := context.WithCancel(ctx)
	defer stopHung()
	for range maxReadsInFlight {
		go client.ReadRange(hung, id, 0, 4)
	}

	// With every slot taken, a read of another pack misses at once rather than
	// costing the local store its whole wait.
	waitFor(t, func() bool { return len(server.readSlots) == maxReadsInFlight })
	missCtx, missDone := context.WithTimeout(ctx, 3*time.Second)
	defer missDone()
	started := time.Now()
	if _, ok := client.ReadRange(missCtx, id, 0, 4); ok || time.Since(started) > time.Second {
		t.Fatalf("read with no slot: ok %v after %v", ok, time.Since(started))
	}

	// A push still answers at once.
	sendCtx, done := context.WithTimeout(ctx, 3*time.Second)
	defer done()
	if err := client.Send(sendCtx, []*block.PutBatchEntry{entry(t, "behind the reads")}); err != nil {
		t.Fatalf("push behind hung reads: %v", err)
	}

	// A new connection gets no fresh slots, so hung reads stay bounded.
	if len(server.readSlots) != maxReadsInFlight {
		t.Fatalf("%d slots taken", len(server.readSlots))
	}

	// The connection still ends.
	finished := make(chan struct{})
	go func() {
		end()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("hung reads held the connection's end")
	}
	for range maxReadsInFlight {
		f, err := os.OpenFile(filepath.Join(dir, "packs", id), os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			_ = f.Close()
		}
	}
}

// nextPending returns the report at the head of the line, due or not.
func nextPending(s *Server) string {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if len(s.pending) == 0 {
		return ""
	}
	return s.pending[0].id
}

// TestReportsOutliveFailures fails a report many times, then lets the local
// store recover: it must still hear the report, on the same connection. A newer
// connection replaces an older one, which then costs the reports nothing.
func TestReportsOutliveFailures(t *testing.T) {
	// A report the local store refuses far past any number of tries.
	ctx := t.Context()
	server, err := NewServer(nopHolder{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server.wait, server.retryBase = time.Minute, time.Microsecond
	if err := server.report(&PackReport{PackId: "pack", Keys: [][]byte{{1}}}); err != nil {
		t.Fatal(err)
	}
	apply := &slowApplier{stuck: "pack"}
	client := NewClient(logrus.NewEntry(logrus.New()), apply)
	defer connect(ctx, t, client, server)()
	waitFor(t, func() bool {
		server.mtx.Lock()
		defer server.mtx.Unlock()
		return server.pending[0].tries > 30
	})

	// A refusal takes its turn at once, not the minute a silent report waits,
	// and recovery is heard. The refusal backs off, so shorten what remains.
	server.mtx.Lock()
	server.pending[0].notBefore = time.Time{}
	server.mtx.Unlock()
	apply.mtx.Lock()
	apply.stuck = ""
	apply.mtx.Unlock()
	server.poke()
	waitFor(t, func() bool { return apply.applied.Load() == 1 })

	// A second connection ends the first.
	near, far := net.Pipe()
	defer near.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, far) }()
	waitFor(t, func() bool {
		client.mtx.Lock()
		defer client.mtx.Unlock()
		return client.out == nil
	})
	_ = near.Close()
	<-done
}

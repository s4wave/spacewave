package block_store_s3

import (
	"bytes"
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/sirupsen/logrus"
)

// countingRemote counts the packs a Drain uploads, and fails them while fail
// is set.
type countingRemote struct {
	*PackStore
	puts atomic.Int32
	fail atomic.Bool
}

// PutPackFile counts the upload, or fails it.
func (r *countingRemote) PutPackFile(ctx context.Context, id string, data []byte) (*packfile.PackfileEntry, error) {
	if r.fail.Load() {
		return nil, errors.New("upload refused")
	}
	r.puts.Add(1)
	return r.PackStore.PutPackFile(ctx, id, data)
}

// TestPackStoreDrain drains packs a packahead Log finalized into an indexed
// store. An upload that fails evicts nothing and the values stay readable
// from the Log; a drain uploads each pack as it is, a second upload changes
// nothing, and past its budget it evicts the uploaded packs, whose blocks a
// store that only lists the bucket reads back with their refs; UploadedBefore
// holds only once the values written before its time are uploaded; and a
// drain of a Log whose packs the store holds uploads none again.
func TestPackStoreDrain(t *testing.T) {
	// Keep the same directory when reopening the Log.
	ctx := t.Context()
	dir := t.TempDir()
	open := func() *packahead.Log {
		log, err := packahead.Open(ctx, dir, packahead.Options{Linger: 10 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		return log
	}
	log := open()
	t.Cleanup(func() { log.Close() })

	// Write a chain of blocks into the Log and wait for its pack.
	var refs []*block.BlockRef
	var datas [][]byte
	appendChain := func(log *packahead.Log, base int) {
		t.Helper()
		var batch []packahead.Block
		for i := range 20 {
			data := bytes.Repeat([]byte{byte(base + i)}, 100+i)
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			var children []*block.BlockRef
			if len(refs) > 0 {
				children = refs[len(refs)-1:]
			}
			value, err := block.EncodeBlockObject(data, children, true)
			if err != nil {
				t.Fatal(err)
			}
			refs, datas = append(refs, ref), append(datas, data)
			batch = append(batch, packahead.Block{Key: packfile.BlockKey(ref.GetHash()), Value: value})
		}
		if err := log.Append(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	waitPacks := func(log *packahead.Log, want int) []packahead.PackInfo {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for {
			infos, waitCh := log.GetPacks()
			if len(infos) == want {
				return infos
			}
			select {
			case <-waitCh:
			case <-deadline:
				t.Fatalf("Log holds %d packs, want %d", len(infos), want)
			}
		}
	}
	appendChain(log, 0)
	written := time.Now()
	infos := waitPacks(log, 1)

	// Open an indexed store on the bucket.
	_, client := newFakeBucket(t, nil)
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	store := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{Index: kv})
	t.Cleanup(store.Close)
	remote := &countingRemote{PackStore: store}

	// A failed upload evicts nothing: the Log still reads every value.
	drain := packahead.NewDrain(log, packahead.DrainOptions{KeepBytes: 1})
	remote.fail.Store(true)
	if err := drain.Run(ctx, remote); err == nil {
		t.Fatal("drain over a failing remote returned nil")
	}
	if drain.UploadedBefore(written) || len(waitPacks(log, 1)) != 1 {
		t.Fatal("failed upload reported uploaded or evicted")
	}
	failed := packahead.DrainStatus{
		Pending: 1, PendingBytes: infos[0].Size, LocalBytes: infos[0].Size,
		Oldest: infos[0].FirstAt, Failures: 1,
	}
	if st, _ := drain.Status(); st != failed {
		t.Fatalf("status after a failed upload = %+v, want %+v", st, failed)
	}
	for _, ref := range refs {
		if !log.Has(packfile.BlockKey(ref.GetHash())) {
			t.Fatalf("Log lost %s after a failed upload", ref.MarshalString())
		}
	}

	// The pack uploads as it is, and again unchanged.
	data, err := os.ReadFile(infos[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		entry, err := store.PutPackFile(ctx, infos[0].ID, data)
		if err != nil {
			t.Fatal(err)
		}
		if entry.GetId() != infos[0].ID || entry.GetBlockCount() != 20 || entry.GetSizeBytes() != uint64(len(data)) {
			t.Fatalf("entry = %v", entry)
		}
	}

	// A second pack drains; past the budget both are evicted.
	appendChain(log, 20)
	later := time.Now()
	infos = waitPacks(log, 2)
	remote.fail.Store(false)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- drain.Run(runCtx, remote) }()
	waitPacks(log, 0)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("stop drain: %v", err)
	}
	if !drain.UploadedBefore(later) {
		t.Fatal("drained Log not reported uploaded")
	}
	// The queue is empty, and the counters retain both attempts.
	drained := packahead.DrainStatus{Written: 1, WrittenBytes: uint64(infos[1].Size), Listed: 1, Failures: 1}
	if st, _ := drain.Status(); st != drained {
		t.Fatalf("status after the drain = %+v, want %+v", st, drained)
	}
	if got := remote.puts.Load(); got != 1 {
		t.Fatalf("drain uploaded %d packs, want the 1 the store lacked", got)
	}
	for _, ref := range refs {
		if found, _, _ := log.Get(packfile.BlockKey(ref.GetHash())); found != nil {
			t.Fatalf("Log still reads evicted %s", ref.MarshalString())
		}
	}

	// A store that lists the bucket reads every block and its refs.
	reader := newTestPackStore(t, client)
	for i, ref := range refs {
		stored, err := reader.GetStoredBlock(ctx, ref)
		if err != nil || stored == nil || !bytes.Equal(stored.Data, datas[i]) {
			t.Fatalf("block %d: %v, %v", i, stored, err)
		}
		if want := min(i, 1); len(stored.Refs) != want || !stored.RefsKnown {
			t.Fatalf("block %d refs: %v known %v", i, stored.Refs, stored.RefsKnown)
		}
	}

	// After a restart, a pack the store holds is not uploaded again.
	log.Close()
	log = open()
	appendChain(log, 40)
	written = time.Now()
	infos = waitPacks(log, 1)
	data, err = os.ReadFile(infos[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutPackFile(ctx, infos[0].ID, data); err != nil {
		t.Fatal(err)
	}
	before := remote.puts.Load()
	drain = packahead.NewDrain(log, packahead.DrainOptions{})
	if drain.UploadedBefore(written) {
		t.Fatal("new drain reported a pack it never saw uploaded")
	}

	// Watch the Log until the drain recognizes the listed pack.
	runCtx, cancel = context.WithCancel(ctx)
	defer cancel()
	go func() { done <- drain.Run(runCtx, remote) }()
	deadline := time.After(10 * time.Second)
	for {
		st, changed := drain.Status()
		if st.Pending == 0 && st.Listed == 1 {
			break
		}
		select {
		case <-changed:
		case err := <-done:
			t.Fatalf("drain stopped before finding the listed pack: %v", err)
		case <-deadline:
			t.Fatalf("drain did not see the listed pack: %+v", st)
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("stop drain after reopen: %v", err)
	}
	if !drain.UploadedBefore(written) {
		t.Fatal("listed pack is still reported pending")
	}
	if got := remote.puts.Load(); got != before || len(waitPacks(log, 1)) != 1 {
		t.Fatalf("drain uploaded %d packs again, or evicted with no budget", got-before)
	}
}

package block_store_writeback

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// packRemote holds the packs a Drain writes.
type packRemote struct {
	mtx   sync.Mutex
	packs map[string][]byte
}

func (r *packRemote) HasPackfile(_ context.Context, id string) (bool, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	_, ok := r.packs[id]
	return ok, nil
}

func (r *packRemote) PutPackFile(_ context.Context, id string, data []byte) (*packfile.PackfileEntry, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.packs[id] = bytes.Clone(data)
	return &packfile.PackfileEntry{Id: id}, nil
}

// TestPackedBatchFillUploads sends a block read from a peer to a diverting
// Store by batch, which carries no cache fill flag: the batch is not marked,
// and its value appends to the Log and uploads in the drained pack, so a
// batched fill the backend lacks still reaches it.
func TestPackedBatchFillUploads(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	s, err := NewStoreWithPacks(ctx, NewPackedStore(log, ownedStore{env.vol}, true), env.vol, env.markers(t), testMarkersID, true)
	if err != nil {
		t.Fatal(err)
	}

	// Store the peer's block by batch.
	data := []byte("a peer's block, stored by batch")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}); err != nil {
		t.Fatal(err)
	}
	if env.isMarked(t, ref) {
		t.Fatal("the batched block has an upload marker under divert")
	}
	if !log.Has(packfile.BlockKey(ref.GetHash())) {
		t.Fatal("the batched block is not in the Log")
	}

	// Drain the Log; the uploaded pack holds the block.
	remote := &packRemote{packs: make(map[string][]byte)}
	d := packahead.NewDrain(log, packahead.DrainOptions{})
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = d.Run(runCtx, remote) }()
	deadline := time.After(10 * time.Second)
	for {
		remote.mtx.Lock()
		var uploaded bool
		for _, pack := range remote.packs {
			uploaded = uploaded || bytes.Contains(pack, data)
		}
		remote.mtx.Unlock()
		if uploaded {
			return
		}
		select {
		case <-time.After(20 * time.Millisecond):
		case <-deadline:
			st, _ := d.Status()
			t.Fatalf("no uploaded pack holds the batched block: %+v", st)
		}
	}
}

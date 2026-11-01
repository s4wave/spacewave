package kvtx

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// txMarks records the blocks a physical transaction stores for the writes that
// asked for marking, in the transaction itself: the marker store lives in the
// same key-value store as the blocks, so a block and its marker commit or
// discard together. Writes that carry no kvtx.BlockMarkRequest are not marked,
// as a refill or an eviction must not queue an upload.
type txMarks struct {
	// store views the physical transaction, and records that a marker write
	// changed it, so an unchanged block still commits its marker.
	store *kvtx.TxStore
	// kvKey locates the marker store's keys.
	kvKey *store_kvkey.KVKey
	// mtx guards done.
	mtx sync.Mutex
	// done holds the markers' commit callbacks, called after the commit.
	done []func()
}

// mark writes the records of blocks through ctx's marker, when ctx carries
// one. Call it after the blocks are stored in the transaction.
func (m *txMarks) mark(ctx context.Context, blocks []kvtx.BlockMark) error {
	// Leave the writes that did not ask for marking.
	req := kvtx.GetBlockMarkRequest(ctx)
	if req == nil || len(blocks) == 0 {
		return nil
	}

	// View the marker store inside the physical transaction.
	marker := req.GetMarker()
	vtx, err := m.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer vtx.Discard()
	tx := kvtx_prefixer.NewTx(vtx, m.kvKey.GetObjectStorePrefixByID(marker.MarkerStoreID()))

	// Write the records, and keep the callback for the commit.
	done, err := marker.MarkBlocks(ctx, tx, blocks)
	if err != nil {
		return err
	}
	req.SetApplied()
	if done != nil {
		m.mtx.Lock()
		m.done = append(m.done, done)
		m.mtx.Unlock()
	}
	return nil
}

// committed calls the markers' commit callbacks. Call it once the transaction
// committed; a discarded transaction drops them.
func (m *txMarks) committed() {
	// Take the callbacks under the lock and run them outside it.
	m.mtx.Lock()
	done := m.done
	m.done = nil
	m.mtx.Unlock()
	for _, fn := range done {
		fn()
	}
}

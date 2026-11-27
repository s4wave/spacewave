package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// diagnosticStore supplies only the read snapshot used by the diagnostics.
type diagnosticStore struct {
	// Store leaves write operations unavailable to these read-only tests.
	Store
	// data is the real transactional store holding blocks and records.
	data kvtx.Store
	// keys encodes block keys alongside the liveness records.
	keys *store_kvkey.KVKey
}

// ViewBlocks exposes one read-only snapshot without allowing a diagnostic to write.
func (s diagnosticStore) ViewBlocks(ctx context.Context, fn func(*Records, block.StoreOps) error) error {
	// Share the underlying reader between records and raw block reads.
	tx, err := s.data.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return fn(NewRecords(tx, []byte("l/")), block_store_kvtx.NewKVTxBlock(s.keys, kvtx.NewTxStore(tx), 0))
}

// _ is a type assertion.
var _ Store = diagnosticStore{}

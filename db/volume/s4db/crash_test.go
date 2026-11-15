//go:build !js && !wasip1

package volume_s4db_test

import (
	"context"
	"testing"

	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume/crashtest"
	"github.com/s4wave/spacewave/db/volume/device"
)

// TestCrashRecovery crashes the volume's block store and key-value store on
// one database at every device call of the crash workload.
func TestCrashRecovery(t *testing.T) {
	crashtest.Run(t, crashtest.ImmediateBlocks, device.NewMemory, func(ctx context.Context, d *device.Memory) (crashtest.Target, error) {
		db, err := s4db.OpenDevice(ctx, d, "volume.s4wave", s4db.Options{})
		if err != nil {
			return nil, err
		}
		return &crashTarget{
			Store:     kvtx_prefixer.NewPrefixer(db, []byte("k/")),
			KVTxBlock: block_store_kvtx.NewKVTxBlock(store_kvkey.NewDefaultKVKey(), db, 0),
			db:        db,
		}, nil
	})
}

// crashTarget is the volume's block store and a key-value store on one
// database.
type crashTarget struct {
	kvtx.Store
	*block_store_kvtx.KVTxBlock

	// db is the database.
	db *s4db.DB
}

// Sync makes every earlier commit durable, as the volume's Sync does.
func (c *crashTarget) Sync(ctx context.Context) (bool, error) {
	return true, c.db.Sync(ctx)
}

// Close closes the database.
func (c *crashTarget) Close() error {
	return c.db.Close()
}

// _ is a type assertion
var _ crashtest.Target = (*crashTarget)(nil)

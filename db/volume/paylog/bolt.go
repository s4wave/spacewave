//go:build !js && !wasip1

package paylog

import (
	"context"

	"github.com/aperturerobotics/bbolt"
	"github.com/pkg/errors"
	kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
	"github.com/s4wave/spacewave/db/volume/device"
)

// boltIndexName is the device file holding the bbolt index.
const boltIndexName = "index"

// boltBucket is the bbolt bucket holding every index key.
var boltBucket = []byte("paylog")

// boltIndex is a bbolt B+tree on one device file. bbolt reads the file into a
// heap buffer at open, and a commit is two device calls: the dirty pages with
// a flush, then the meta page with a flush.
type boltIndex struct {
	*kvtx_bolt.Store
}

// OpenBolt opens the bbolt index on dev, creating it when dev has none.
func OpenBolt(ctx context.Context, dev device.Device) (Index, error) {
	h, err := device.OpenHandle(ctx, dev, boltIndexName)
	if err != nil {
		return nil, err
	}
	db, err := bbolt.OpenStorage(h, &bbolt.Options{PageSize: 4096, NoGrowSync: true})
	if err != nil {
		return nil, errors.Wrap(err, "open index")
	}

	// Create the bucket on a new index.
	var found bool
	err = db.View(func(tx *bbolt.Tx) error {
		found = tx.Bucket(boltBucket) != nil
		return nil
	})
	if err == nil && !found {
		err = db.Update(func(tx *bbolt.Tx) error {
			_, err := tx.CreateBucket(boltBucket)
			return err
		})
	}
	if err != nil {
		_ = db.Close()
		return nil, errors.Wrap(err, "init index")
	}
	return &boltIndex{Store: kvtx_bolt.NewStore(db, boltBucket)}, nil
}

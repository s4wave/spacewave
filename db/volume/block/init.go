package volume_block

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/sirupsen/logrus"
)

// InitVolume initializes a new volume w/ a private key.
//
// Uses the transform config from the cursor.
func InitVolume(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
	cursor *bucket_lookup.Cursor,
	nvolPriv crypto.PrivKey,
) (*bucket.ObjectRef, error) {
	// Build the kvtx block store.
	wrefCh := make(chan *bucket.ObjectRef, 1)
	commitFn := func(nref *bucket.ObjectRef) error {
		select {
		case wrefCh <- nref:
			return nil
		default:
			return errors.New("expected only one commit")
		}
	}

	cursor.SetRootRef(nil)
	bstore, err := kvtx_block.NewStore(ctx, le, cursor, commitFn)
	if err != nil {
		return nil, err
	}

	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	hstore := store_kvtx.NewKVTx(kvkey, bstore, conf.GetStoreConfig())
	err = hstore.StorePeerPriv(ctx, nvolPriv)
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, context.Canceled
	case nrootRef := <-wrefCh:
		return nrootRef, nil
	}
}

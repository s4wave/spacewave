package packmove

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// requireFormat returns an error unless the volume's format keeps a child list
// for every block and is one this build reads.
func requireFormat(ctx context.Context, cfg PackConfig) error {
	version, err := readVersion(ctx, cfg.Source, cfg.Keys)
	if err != nil {
		return err
	}
	if version < minFormatVersion || version > store_kvkey.FormatVersion {
		return errors.Errorf("pack needs volume format %d to %d, found %d; copy it to the current format first", minFormatVersion, store_kvkey.FormatVersion, version)
	}
	return nil
}

// readVersion returns the volume's format version, or 0 when unset.
func readVersion(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey) (uint64, error) {
	var version uint64
	err := readStable(ctx, store, func(tx kvtx.Tx) error {
		data, found, err := tx.Get(ctx, keys.GetFormatVersionKey())
		if err != nil || !found {
			version = 0
			return err
		}
		version, err = store_kvkey.ParseFormatVersion(data)
		return err
	})
	return version, err
}

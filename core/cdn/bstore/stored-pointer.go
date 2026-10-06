package cdn_bstore

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/cdn"
	"github.com/s4wave/spacewave/db/kvtx"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
)

// storedPointerKey is the Options.PointerStore key of the last fetched root
// pointer.
var storedPointerKey = []byte("root_pointer")

// stalePointer returns the pointer to serve after fetching the root pointer
// failed with fetchErr: the cached pointer, else the stored one. Serving it
// keeps cached blocks of the last fetched manifest readable while the CDN is
// unreachable, and renews its fetch time so the next fetch waits for the TTL.
func (s *CdnBlockStore) stalePointer(ctx context.Context, cached *cdn.CdnRootPointer, fetchErr error) (*cdn.CdnRootPointer, error) {
	// A canceled read or a closed store has no reader to serve.
	if ctx.Err() != nil || errors.Is(fetchErr, packfile_store.ErrPackfileStoreClosed) {
		return nil, fetchErr
	}

	// Prefer the pointer this store fetched, then the stored one.
	ptr := cached
	if ptr == nil {
		var err error
		ptr, err = s.readStoredPointer(ctx)
		if err != nil || ptr == nil {
			return nil, fetchErr
		}
	}
	if _, published := s.setPointer(ctx, ptr); !published {
		return nil, packfile_store.ErrPackfileStoreClosed
	}
	return ptr, nil
}

// readStoredPointer returns the stored root pointer, or nil if none is stored.
func (s *CdnBlockStore) readStoredPointer(ctx context.Context) (*cdn.CdnRootPointer, error) {
	// Without a pointer store, nothing is stored.
	store := s.opts.PointerStore
	if store == nil {
		return nil, nil
	}

	// Copy the stored pointer out of the transaction.
	var data []byte
	err := kvtx.RunTransaction(ctx, false, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, false)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		value, found, err := tx.Get(ctx, storedPointerKey)
		if err != nil || !found {
			return err
		}
		data = bytes.Clone(value)
		return nil
	})
	if err != nil || data == nil {
		return nil, err
	}

	// Decode the pointer.
	ptr := &cdn.CdnRootPointer{}
	if err := ptr.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode stored root pointer")
	}
	return ptr, nil
}

// writeStoredPointer stores ptr as the last fetched root pointer.
func (s *CdnBlockStore) writeStoredPointer(ctx context.Context, ptr *cdn.CdnRootPointer) error {
	// Encode the pointer when there is a store to keep it.
	store := s.opts.PointerStore
	if store == nil || ptr == nil {
		return nil
	}
	data, err := ptr.MarshalVT()
	if err != nil {
		return err
	}

	// Replace the stored pointer.
	return kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, storedPointerKey, data)
	})
}

package provider_local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"slices"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
)

// CreateArrivingSharedObject creates the sobject id as CreateSharedObject
// does, with a SharedObjectOrigin holding source and marked arriving, stored
// in the transaction that lists it. The account refuses to mount the object
// with ErrSharedObjectArriving until ReplaceSharedObjectState writes its
// state; a delete still removes it. When the account already lists id it
// creates nothing and returns ErrSharedObjectExists.
func (a *ProviderAccount) CreateArrivingSharedObject(ctx context.Context, id string, meta *sobject.SharedObjectMeta, source []byte) error {
	_, err := a.createSharedObject(ctx, id, meta, func(ctx context.Context, tx kvtx.Tx, entry *sobject.SharedObjectListEntry) error {
		return writeSharedObjectOrigin(ctx, tx, id, &SharedObjectOrigin{
			Generation: entry.GetGeneration(),
			Arriving:   true,
			Source:     source,
		})
	})
	return err
}

// StoredSharedObject is what the account stores for a shared object, read
// in one transaction by ReadStoredSharedObject.
type StoredSharedObject struct {
	// Entry is the list entry, nil when the account does not list the object.
	Entry *sobject.SharedObjectListEntry
	// Origin is the origin stored for the listed generation, nil when there is
	// none or it belongs to an earlier lifetime of the id.
	Origin *SharedObjectOrigin
	// State is the stored host state, nil when none is stored.
	State []byte
}

// Unchanged reports whether the stored state is the one the origin's writer
// stored: the origin is written and the state hashes to its StateHash.
func (s *StoredSharedObject) Unchanged() bool {
	if s.Origin == nil || s.Origin.GetArriving() || len(s.Origin.GetStateHash()) == 0 || s.State == nil {
		return false
	}
	sum := sha256.Sum256(s.State)
	return bytes.Equal(sum[:], s.Origin.GetStateHash())
}

// ReadStoredSharedObject reads the list entry, origin and host state of the
// shared object id in one transaction.
func (a *ProviderAccount) ReadStoredSharedObject(ctx context.Context, id string) (*StoredSharedObject, error) {
	out := &StoredSharedObject{}
	err := a.runSoObjectStoreTx(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		// Read the entry and its origin; an unlisted id has neither.
		*out = StoredSharedObject{}
		entry, origin, err := readListedOrigin(ctx, tx, id)
		if err != nil || entry == nil {
			return err
		}
		out.Entry, out.Origin = entry, origin

		// Read the stored state in the same transaction.
		data, found, err := tx.Get(ctx, SobjectObjectStoreHostStateKey(id))
		if found {
			out.State = slices.Clone(data)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// readListedOrigin returns the stored list entry of id, nil when unlisted,
// and its origin, nil when none is stored for the entry's generation.
func readListedOrigin(ctx context.Context, tx kvtx.Tx, id string) (*sobject.SharedObjectListEntry, *SharedObjectOrigin, error) {
	// Find the listed entry.
	list, err := readStoredSharedObjectList(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	var entry *sobject.SharedObjectListEntry
	for _, e := range list.GetSharedObjects() {
		if e.GetRef().GetProviderResourceRef().GetId() == id {
			entry = e
			break
		}
	}
	if entry == nil {
		return nil, nil, nil
	}

	// Read its origin, ignoring one an earlier lifetime of the id left.
	data, found, err := tx.Get(ctx, SobjectOriginKey(id))
	if err != nil || !found {
		return entry, nil, err
	}
	origin := &SharedObjectOrigin{}
	if err := origin.UnmarshalVT(data); err != nil {
		return nil, nil, err
	}
	if origin.GetGeneration() != entry.GetGeneration() {
		return entry, nil, nil
	}
	return entry, origin, nil
}

// writeSharedObjectOrigin stores origin as the origin of id.
func writeSharedObjectOrigin(ctx context.Context, tx kvtx.Tx, id string, origin *SharedObjectOrigin) error {
	data, err := origin.MarshalVT()
	if err != nil {
		return err
	}
	return tx.Set(ctx, SobjectOriginKey(id), data)
}

// refuseArriving returns ErrSharedObjectArriving when the listed object id
// holds an arriving origin. It reads the list only when an origin is stored.
func refuseArriving(ctx context.Context, tx kvtx.Tx, id string) error {
	// Skip the list read when no origin is stored.
	found, err := tx.Exists(ctx, SobjectOriginKey(id))
	if err != nil || !found {
		return err
	}

	// Refuse while the listed generation's origin is arriving.
	_, origin, err := readListedOrigin(ctx, tx, id)
	if err != nil {
		return err
	}
	if origin.GetArriving() {
		return sobject.ErrSharedObjectArriving
	}
	return nil
}

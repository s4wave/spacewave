package resource_state

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
)

// ObjectStoreStateAtom implements StateAtomStore using an ObjectStore.
type ObjectStoreStateAtom struct {
	storeID  string
	objStore object.ObjectStore
	objKey   []byte

	bcast broadcast.Broadcast
	seqno atomic.Uint64
}

// NewObjectStoreStateAtom creates a new ObjectStore-backed state atom.
func NewObjectStoreStateAtom(
	storeID string,
	objStore object.ObjectStore,
) *ObjectStoreStateAtom {
	return &ObjectStoreStateAtom{
		storeID:  storeID,
		objStore: objStore,
		objKey:   []byte("state/" + storeID),
	}
}

// GetStoreID returns the store ID.
func (s *ObjectStoreStateAtom) GetStoreID() string {
	return s.storeID
}

// Get returns the current state JSON and sequence number.
func (s *ObjectStoreStateAtom) Get(ctx context.Context) (string, uint64, error) {
	var state string
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return s.objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			data, found, err := tx.Get(ctx, s.objKey)
			if err != nil {
				return err
			}
			if !found {
				state = "{}"
				return nil
			}
			state = string(data)
			return nil
		},
	)
	return state, s.seqno.Load(), err
}

// Set updates the state JSON and returns the new sequence number.
func (s *ObjectStoreStateAtom) Set(ctx context.Context, stateJson string) (uint64, error) {
	err := kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return s.objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, s.objKey, []byte(stateJson))
		},
	)
	if err != nil {
		return 0, err
	}

	// Increment seqno and broadcast change
	var newSeqno uint64
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		newSeqno = s.seqno.Add(1)
		broadcast()
	})
	return newSeqno, nil
}

// WaitSeqno blocks until the seqno is >= the given value.
func (s *ObjectStoreStateAtom) WaitSeqno(ctx context.Context, minSeqno uint64) (uint64, error) {
	for {
		var currSeqno uint64
		var waitCh <-chan struct{}

		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			currSeqno = s.seqno.Load()
			if currSeqno < minSeqno {
				waitCh = getWaitCh()
			}
		})

		if currSeqno >= minSeqno {
			return currSeqno, nil
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-waitCh:
		}
	}
}

// _ is a type assertion
var _ StateAtomStore = (*ObjectStoreStateAtom)(nil)

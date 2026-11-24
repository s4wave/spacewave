package provider_local

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_statestore "github.com/s4wave/spacewave/core/sobject/statestore"
	"github.com/s4wave/spacewave/db/kvtx"
)

// stateLayoutLog selects the log layout (sobject_statestore.LayoutEnv) over
// the whole state under the host key. It is read once, here, at startup.
//
// Invariants of the two layouts and the conversion between them:
//
//  1. A committed state is in exactly one layout: a conversion writes the
//     selected layout and clears the other in one transaction.
//  2. A conversion commits only after the state loaded back from the
//     selected layout equals the state decoded from the other one, read in
//     the same transaction. A failed check stores nothing and fails the
//     mount.
//  3. The token key's bytes change with every committed write, so a write's
//     compare and its settle never mistake another write for this one.
//  4. Every state/ key under an object belongs to its current lineage: a
//     replace deletes every key it does not write, since a load assembles
//     whatever the keys hold.
//  5. Only the selected layout is written. A mount converts a state stored in
//     the other layout before its first write, in either direction, so
//     switching back loses no write made under the log.
var stateLayoutLog = sobject_statestore.LogSelected()

// errStateLayoutMismatch is returned when a state converted to the selected
// layout does not load back equal to the state it was converted from.
var errStateLayoutMismatch = errors.New("shared object state differs after converting its layout")

// soStateLayout is one way of storing a SharedObject's state.
type soStateLayout interface {
	// tokenKey returns the key whose bytes every write compares and changes.
	// The state is stored in this layout when it exists.
	tokenKey() []byte
	// load reads the state and the token key's bytes.
	load(ctx context.Context, tx kvtx.TxOps) (state *sobject.SOState, token []byte, found bool, err error)
	// write stores next over prev, the state stored under token, and returns
	// the new token. The caller compared the stored token first.
	write(ctx context.Context, tx kvtx.TxOps, token []byte, prev, next *sobject.SOState) ([]byte, error)
	// replace stores state as a new lineage and returns its token.
	replace(ctx context.Context, tx kvtx.TxOps, state *sobject.SOState) ([]byte, error)
	// clear deletes every key of the layout.
	clear(ctx context.Context, tx kvtx.TxOps) error
}

// soStateLayouts returns the selected layout of id's state and the other one.
func soStateLayouts(id string) (selected, other soStateLayout) {
	whole, log := soStateLayoutPair(id)
	if stateLayoutLog {
		return log, whole
	}
	return whole, log
}

// soStateLayoutPair returns both layouts of id's state.
func soStateLayoutPair(id string) (wholeLayout, logLayout) {
	return wholeLayout(SobjectObjectStoreHostStateKey(id)), logLayout{sobject_statestore.New(SobjectObjectStorePrefix(id))}
}

// soStateKeys returns the token keys of both layouts of id's state.
func soStateKeys(id string) [2][]byte {
	selected, other := soStateLayouts(id)
	return [2][]byte{selected.tokenKey(), other.tokenKey()}
}

// soStateStored reports whether id's state is stored in either layout.
func soStateStored(ctx context.Context, tx kvtx.TxOps, id string) (bool, error) {
	for _, key := range soStateKeys(id) {
		if found, err := tx.Exists(ctx, key); err != nil || found {
			return found, err
		}
	}
	return false, nil
}

// readSOStateData returns the encoded state of id in either layout: the
// bytes the whole layout stores, or the encoding of the state the log
// assembles, which equals the bytes that state was written from.
func readSOStateData(ctx context.Context, tx kvtx.TxOps, id string) ([]byte, bool, error) {
	// The whole layout stores the encoding itself.
	whole, log := soStateLayoutPair(id)
	data, found, err := tx.Get(ctx, whole)
	if err != nil || found {
		return bytes.Clone(data), found, err
	}

	// Encode the state the log assembles.
	state, _, found, err := log.load(ctx, tx)
	if err != nil || !found {
		return nil, found, err
	}
	data, err = state.MarshalVT()
	return data, err == nil, err
}

// replaceSOState stores state as id's new lineage in the selected layout and
// clears the other one.
func replaceSOState(ctx context.Context, tx kvtx.TxOps, id string, state *sobject.SOState) error {
	selected, other := soStateLayouts(id)
	if err := other.clear(ctx, tx); err != nil {
		return err
	}
	_, err := selected.replace(ctx, tx, state)
	return err
}

// convertSOState moves id's state into the selected layout when it is stored
// in the other one, following the invariants on stateLayoutLog, and returns
// the converted state as the selected layout loads it, or nil when it wrote
// nothing.
func convertSOState(ctx context.Context, tx kvtx.TxOps, id string) (*sobject.SOState, error) {
	// A state already in the selected layout, or none, needs nothing.
	selected, other := soStateLayouts(id)
	if found, err := tx.Exists(ctx, selected.tokenKey()); err != nil || found {
		return nil, err
	}
	state, _, found, err := other.load(ctx, tx)
	if err != nil || !found {
		return nil, err
	}

	// Write the selected layout and check it loads back equal.
	if _, err := selected.replace(ctx, tx, state); err != nil {
		return nil, err
	}
	converted, _, found, err := selected.load(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !found || !converted.EqualVT(state) {
		return nil, errors.Wrap(errStateLayoutMismatch, id)
	}

	// Clear the other layout in the same transaction.
	return converted, other.clear(ctx, tx)
}

// wholeLayout stores the encoded state under one key, the token key.
type wholeLayout []byte

// tokenKey returns the state's key.
func (l wholeLayout) tokenKey() []byte {
	return l
}

// load decodes the state; its encoding is the token.
func (l wholeLayout) load(ctx context.Context, tx kvtx.TxOps) (*sobject.SOState, []byte, bool, error) {
	// Read the encoding, then decode a copy the transaction does not own.
	data, found, err := tx.Get(ctx, l)
	if err != nil || !found {
		return nil, nil, found, err
	}
	data = bytes.Clone(data)
	state := &sobject.SOState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, nil, false, err
	}
	return state, data, true, nil
}

// write encodes and stores next whole.
func (l wholeLayout) write(ctx context.Context, tx kvtx.TxOps, _ []byte, _, next *sobject.SOState) ([]byte, error) {
	return l.replace(ctx, tx, next)
}

// replace encodes and stores state.
func (l wholeLayout) replace(ctx context.Context, tx kvtx.TxOps, state *sobject.SOState) ([]byte, error) {
	data, err := state.MarshalVT()
	if err != nil {
		return nil, err
	}
	return data, tx.Set(ctx, l, data)
}

// clear deletes the state's key.
func (l wholeLayout) clear(ctx context.Context, tx kvtx.TxOps) error {
	return tx.Delete(ctx, l)
}

// logLayout stores the state as a head and its parts; the head is the token.
type logLayout struct {
	sobject_statestore.Store
}

// tokenKey returns the head's key.
func (l logLayout) tokenKey() []byte {
	return l.HeadKey()
}

// load reads the state and its head.
func (l logLayout) load(ctx context.Context, tx kvtx.TxOps) (*sobject.SOState, []byte, bool, error) {
	return l.Load(ctx, tx)
}

// write stores the delta from prev to next.
func (l logLayout) write(ctx context.Context, tx kvtx.TxOps, token []byte, prev, next *sobject.SOState) ([]byte, error) {
	return l.Write(ctx, tx, token, prev, next)
}

// replace stores state as a new lineage.
func (l logLayout) replace(ctx context.Context, tx kvtx.TxOps, state *sobject.SOState) ([]byte, error) {
	return l.Replace(ctx, tx, state)
}

// clear deletes the head and every part.
func (l logLayout) clear(ctx context.Context, tx kvtx.TxOps) error {
	return l.Clear(ctx, tx)
}

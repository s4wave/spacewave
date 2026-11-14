package provider_local

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"os"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
)

// StateLayoutEnv names the environment variable that selects how a local
// shared object's state is stored, read once at startup. "split" stores the
// state in parts, so an operation writes only itself and the head; any other
// value keeps the whole encoding under the one host key. A mount converts its
// object to the selected layout as it loads, so unsetting the variable and
// restarting carries split objects back to the host key.
const StateLayoutEnv = "SPACEWAVE_SOBJECT_STATE_LAYOUT"

// splitStateOn selects the split layout. It is read once, here, at startup.
var splitStateOn = os.Getenv(StateLayoutEnv) == "split"

// The split layout keeps a shared object's state under so/<id>/st/:
//
//   - h holds the head: a generation drawn at random for each write. The head
//     exists exactly when the split layout holds the state.
//   - b holds the base: the state without its checkpoint, operations and
//     sequence.
//   - c holds the checkpoint, absent when the state has none.
//   - o/<hash> holds each operation, keyed by its hash.
//   - s/<height, 8 bytes big endian><hash> holds each sequence record.
//
// Operations are held sorted by hash and sequence records by height, then
// hash, so a sorted scan of each prefix yields them in the state's order.

// splitStatePrefix returns the prefix of every split layout key of id.
func splitStatePrefix(id string) []byte {
	return []byte("so/" + id + "/st/")
}

// splitStateHeadKey returns the key of the head of id's split state.
func splitStateHeadKey(id string) []byte {
	return append(splitStatePrefix(id), 'h')
}

// splitStateBaseKey returns the key of the base of id's split state.
func splitStateBaseKey(id string) []byte {
	return append(splitStatePrefix(id), 'b')
}

// splitStateCheckpointKey returns the key of the checkpoint of id's split state.
func splitStateCheckpointKey(id string) []byte {
	return append(splitStatePrefix(id), 'c')
}

// splitStateOpPrefix returns the prefix of the operation keys of id.
func splitStateOpPrefix(id string) []byte {
	return append(splitStatePrefix(id), 'o', '/')
}

// splitStateSeqPrefix returns the prefix of the sequence record keys of id.
func splitStateSeqPrefix(id string) []byte {
	return append(splitStatePrefix(id), 's', '/')
}

// splitStateGenLen is the length of a split state generation.
const splitStateGenLen = 16

// errSplitStateBroken is returned when a split state's head has no base.
var errSplitStateBroken = errors.New("split shared object state has a head but no base")

// errStateOrder is returned for a write whose operations or sequence records
// are out of order.
var errStateOrder = errors.New("shared object state entries are out of order")

// soStateStamp identifies one stored version of a shared object's state: its
// whole encoding under the host key, or its generation under the split layout.
type soStateStamp struct {
	// split marks a stamp of the split layout.
	split bool
	// v is the encoding or the generation.
	v []byte
}

// equal reports whether s and o identify the same stored version.
func (s soStateStamp) equal(o soStateStamp) bool {
	return s.split == o.split && bytes.Equal(s.v, o.v)
}

// newSplitStamp draws the stamp of a new split state version.
func newSplitStamp() soStateStamp {
	gen := make([]byte, splitStateGenLen)
	_, _ = rand.Read(gen)
	return soStateStamp{split: true, v: gen}
}

// readSOStateStamp reads the stamp of id's stored state, in either layout.
func readSOStateStamp(ctx context.Context, tx kvtx.TxOps, id string) (soStateStamp, bool, error) {
	gen, found, err := tx.Get(ctx, splitStateHeadKey(id))
	if err != nil || found {
		return soStateStamp{split: true, v: bytes.Clone(gen)}, found, err
	}
	data, found, err := tx.Get(ctx, SobjectObjectStoreHostStateKey(id))
	return soStateStamp{v: data}, found, err
}

// soStateExists reports whether id has stored state in either layout.
func soStateExists(ctx context.Context, tx kvtx.TxOps, id string) (bool, error) {
	found, err := tx.Exists(ctx, splitStateHeadKey(id))
	if err != nil || found {
		return found, err
	}
	return tx.Exists(ctx, SobjectObjectStoreHostStateKey(id))
}

// readSOState reads and decodes id's stored state in either layout, with its
// stamp. It does not validate the state.
func readSOState(ctx context.Context, tx kvtx.TxOps, id string) (*sobject.SOState, soStateStamp, bool, error) {
	// Read the stamp, which names the layout.
	stamp, found, err := readSOStateStamp(ctx, tx, id)
	if err != nil || !found {
		return nil, stamp, found, err
	}

	// Assemble the parts or decode the host key's encoding.
	var state *sobject.SOState
	if stamp.split {
		state, err = readSplitSOState(ctx, tx, id)
	} else {
		state = &sobject.SOState{}
		err = state.UnmarshalVT(stamp.v)
	}
	if err != nil {
		return nil, stamp, true, err
	}
	return state, stamp, true, nil
}

// readSOStateEncoded reads the encoding of id's stored state in either layout:
// the bytes the host key holds or would hold.
func readSOStateEncoded(ctx context.Context, tx kvtx.TxOps, id string) ([]byte, bool, error) {
	state, stamp, found, err := readSOState(ctx, tx, id)
	if err != nil || !found || !stamp.split {
		return stamp.v, found, err
	}
	data, err := state.MarshalVT()
	return data, true, err
}

// readSplitSOState assembles id's split state from its parts.
func readSplitSOState(ctx context.Context, tx kvtx.TxOps, id string) (*sobject.SOState, error) {
	// Read the base and the checkpoint.
	base, found, err := tx.Get(ctx, splitStateBaseKey(id))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errSplitStateBroken
	}
	state := &sobject.SOState{}
	if err := state.UnmarshalVT(base); err != nil {
		return nil, err
	}
	cp, found, err := tx.Get(ctx, splitStateCheckpointKey(id))
	if err != nil {
		return nil, err
	}
	if found {
		state.Checkpoint = &sobject.SOCheckpoint{}
		if err := state.Checkpoint.UnmarshalVT(cp); err != nil {
			return nil, err
		}
	}

	// Scan the operations and sequence records in key order.
	err = scanSorted(ctx, tx, splitStateOpPrefix(id), func(v []byte) error {
		op := &sobject.SOOperation{}
		if err := op.UnmarshalVT(v); err != nil {
			return err
		}
		state.Ops = append(state.Ops, op)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = scanSorted(ctx, tx, splitStateSeqPrefix(id), func(v []byte) error {
		rec := &sobject.SOSequence{}
		if err := rec.UnmarshalVT(v); err != nil {
			return err
		}
		state.Sequence = append(state.Sequence, rec)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return state, nil
}

// scanSorted calls cb with each value under prefix in key order. cb must not
// retain the value.
func scanSorted(ctx context.Context, tx kvtx.TxOps, prefix []byte, cb func(v []byte) error) error {
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next(); it.Valid(); it.Next() {
		v, err := it.Value()
		if err != nil {
			return err
		}
		if err := cb(v); err != nil {
			return err
		}
	}
	return it.Err()
}

// writeSOState writes next over prev, the state stamp names, in stamp's
// layout, and returns the new stamp. Under the host key it stores data, the
// encoding of next. Under the split layout it writes the head with a fresh
// generation and only the parts next changes, at a cost of one compare per
// held entry and a hash per changed one. The caller checked in tx that
// storage holds stamp.
//
// Invariants:
//   - Each write of the head, base, checkpoint, operation and sequence keys
//     happens in one transaction with the compare that admits it, so a crash
//     leaves the old state or the new one, never a mix.
//   - The state loaded from the parts encodes to the same bytes as the state
//     written. The writes rely on EqualVT telling every difference in
//     encoding, including nil from empty, and refuse a state whose operations
//     or sequence records are out of key order, which the parts could not
//     hold in that order.
//   - The host key and the head are never both stored: a conversion deletes
//     the layout it leaves in the transaction that writes the other, and a
//     fresh write deletes both first.
//   - A writer commits only when the stored stamp, the whole encoding or the
//     generation, is the one it loaded or last wrote; a stale writer writes
//     nothing.
func writeSOState(ctx context.Context, tx kvtx.Tx, id string, stamp soStateStamp, prev, next *sobject.SOState, data []byte) (soStateStamp, error) {
	if !stamp.split {
		return soStateStamp{v: data}, tx.Set(ctx, SobjectObjectStoreHostStateKey(id), data)
	}
	out := newSplitStamp()
	return out, writeSplitSOState(ctx, tx, id, out, prev, next)
}

// writeSplitSOState writes the parts of next that differ from prev, nil for
// none stored, under the head stamp.
func writeSplitSOState(ctx context.Context, tx kvtx.Tx, id string, stamp soStateStamp, prev, next *sobject.SOState) error {
	// Write the base and checkpoint when they changed.
	nextBase := splitStateBase(next)
	if prev == nil || !splitStateBase(prev).EqualVT(nextBase) {
		if err := setVT(ctx, tx, splitStateBaseKey(id), nextBase); err != nil {
			return err
		}
	}
	cp := next.GetCheckpoint()
	if prev == nil || !prev.GetCheckpoint().EqualVT(cp) {
		var err error
		if cp == nil {
			err = tx.Delete(ctx, splitStateCheckpointKey(id))
		} else {
			err = setVT(ctx, tx, splitStateCheckpointKey(id), cp)
		}
		if err != nil {
			return err
		}
	}

	// Write the added and changed entries, and delete the dropped ones.
	opPrefix := splitStateOpPrefix(id)
	opKey := func(op *sobject.SOOperation) ([]byte, error) {
		return append(bytes.Clone(opPrefix), op.Hash()...), nil
	}
	err := diffSorted(ctx, tx, prev.GetOps(), next.GetOps(), opKey)
	if err != nil {
		return err
	}
	seqPrefix := splitStateSeqPrefix(id)
	seqKey := func(rec *sobject.SOSequence) ([]byte, error) {
		height, err := rec.UnmarshalHeight()
		if err != nil {
			return nil, err
		}
		key := binary.BigEndian.AppendUint64(bytes.Clone(seqPrefix), height)
		return append(key, rec.Hash()...), nil
	}
	if err := diffSorted(ctx, tx, prev.GetSequence(), next.GetSequence(), seqKey); err != nil {
		return err
	}

	// Commit the version with the head.
	return tx.Set(ctx, splitStateHeadKey(id), stamp.v)
}

// splitStateBase returns state without the parts stored under their own keys.
func splitStateBase(state *sobject.SOState) *sobject.SOState {
	base := *state
	base.Checkpoint, base.Ops, base.Sequence = nil, nil, nil
	return &base
}

// vtMessage is a message diffSorted can compare, encode and store.
type vtMessage[T any] interface {
	// EqualVT reports whether the message encodes as the other does.
	EqualVT(T) bool
	// MarshalVT encodes the message.
	MarshalVT() ([]byte, error)
}

// setVT stores the encoding of msg at key.
func setVT(ctx context.Context, tx kvtx.Tx, key []byte, msg interface{ MarshalVT() ([]byte, error) }) error {
	data, err := msg.MarshalVT()
	if err != nil {
		return err
	}
	return tx.Set(ctx, key, data)
}

// diffSorted stores the entries of next that prev lacks or holds with another
// encoding, and deletes the keys of the entries of prev that next lacks. prev
// and next are sorted by key. Entries equal at the walk position are passed
// without computing a key, so the walk costs one compare per held entry and
// keys only around each change. Since the unchanged entries keep prev's
// order, next is sorted exactly when each stored entry sorts strictly between
// its neighbors, which the walk checks, failing with errStateOrder.
func diffSorted[T vtMessage[T]](ctx context.Context, tx kvtx.Tx, prev, next []T, key func(T) ([]byte, error)) error {
	// Walk both in key order, recording the indexes of next it stores.
	var stored []int
	i, j := 0, 0
	for i < len(prev) || j < len(next) {
		// Pass an entry both hold.
		if i < len(prev) && j < len(next) && prev[i].EqualVT(next[j]) {
			i, j = i+1, j+1
			continue
		}

		// Order the two entries at the walk position by key.
		var pk, nk []byte
		var err error
		if i < len(prev) {
			if pk, err = key(prev[i]); err != nil {
				return err
			}
		}
		if j < len(next) {
			if nk, err = key(next[j]); err != nil {
				return err
			}
		}
		var c int
		switch {
		case pk == nil:
			c = 1
		case nk == nil:
			c = -1
		default:
			c = bytes.Compare(pk, nk)
		}

		// Delete a dropped entry; store an added or changed one.
		if c < 0 {
			if err := tx.Delete(ctx, pk); err != nil {
				return err
			}
			i++
			continue
		}
		if err := setVT(ctx, tx, nk, next[j]); err != nil {
			return err
		}
		stored = append(stored, j)
		if c == 0 {
			i++
		}
		j++
	}

	// Check that each stored entry sorts between its neighbors.
	for _, s := range stored {
		k, err := key(next[s])
		if err != nil {
			return err
		}
		for _, n := range []int{s - 1, s + 1} {
			if n < 0 || n >= len(next) {
				continue
			}
			nk, err := key(next[n])
			if err != nil {
				return err
			}
			if c := bytes.Compare(nk, k); c == 0 || (c > 0) != (n > s) {
				return errStateOrder
			}
		}
	}
	return nil
}

// storeNewSOState stores state, whose encoding is data, as id's whole state in
// the layout splitStateOn selects, deleting any state stored in either layout
// first, and returns its stamp.
func storeNewSOState(ctx context.Context, tx kvtx.Tx, id string, state *sobject.SOState, data []byte) (soStateStamp, error) {
	if err := deleteSOState(ctx, tx, id); err != nil {
		return soStateStamp{}, err
	}
	if !splitStateOn {
		return soStateStamp{v: data}, tx.Set(ctx, SobjectObjectStoreHostStateKey(id), data)
	}
	stamp := newSplitStamp()
	return stamp, writeSplitSOState(ctx, tx, id, stamp, nil, state)
}

// deleteSOState deletes id's stored state in both layouts.
func deleteSOState(ctx context.Context, tx kvtx.Tx, id string) error {
	if err := tx.Delete(ctx, SobjectObjectStoreHostStateKey(id)); err != nil {
		return err
	}
	return deleteKeysWithPrefix(ctx, tx, splitStatePrefix(id))
}

// convertSOState moves id's stored state, read as state under stamp, to the
// layout splitStateOn selects, in tx, and returns the new stamp. The caller
// checked in tx that storage holds stamp. The layout it leaves is deleted only
// in tx, with every key of the new one written. A host key state goes to the
// split layout only when the parts load back to its exact bytes; otherwise it
// stays where it is and the stamp is unchanged.
func convertSOState(ctx context.Context, tx kvtx.Tx, id string, state *sobject.SOState, stamp soStateStamp) (soStateStamp, error) {
	// A state already in the selected layout stays.
	if stamp.split == splitStateOn {
		return stamp, nil
	}

	// Any split state goes back to the host key as its encoding.
	if stamp.split {
		data, err := state.MarshalVT()
		if err != nil {
			return stamp, err
		}
		return storeNewSOState(ctx, tx, id, state, data)
	}

	// A host key state splits only when the parts load back to its bytes.
	same, err := splitRoundTrips(state, stamp.v)
	if err != nil || !same {
		return stamp, err
	}
	return storeNewSOState(ctx, tx, id, state, stamp.v)
}

// splitRoundTrips reports whether state, split into parts as the split layout
// stores them and assembled as a load assembles them, encodes to data.
func splitRoundTrips(state *sobject.SOState, data []byte) (bool, error) {
	// Round-trip the base and the checkpoint.
	out := &sobject.SOState{}
	if err := roundTripVT(splitStateBase(state), out); err != nil {
		return false, err
	}
	if cp := state.GetCheckpoint(); cp != nil {
		out.Checkpoint = &sobject.SOCheckpoint{}
		if err := roundTripVT(cp, out.Checkpoint); err != nil {
			return false, err
		}
	}

	// Round-trip each operation and sequence record.
	for _, op := range state.GetOps() {
		o := &sobject.SOOperation{}
		if err := roundTripVT(op, o); err != nil {
			return false, err
		}
		out.Ops = append(out.Ops, o)
	}
	for _, rec := range state.GetSequence() {
		r := &sobject.SOSequence{}
		if err := roundTripVT(rec, r); err != nil {
			return false, err
		}
		out.Sequence = append(out.Sequence, r)
	}

	// Compare the assembled encoding with the stored bytes.
	got, err := out.MarshalVT()
	if err != nil {
		return false, err
	}
	return bytes.Equal(got, data), nil
}

// roundTripVT decodes the encoding of in into out.
func roundTripVT(in interface{ MarshalVT() ([]byte, error) }, out interface{ UnmarshalVT([]byte) error }) error {
	data, err := in.MarshalVT()
	if err != nil {
		return err
	}
	return out.UnmarshalVT(data)
}

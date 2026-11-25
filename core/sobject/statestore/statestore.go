// Package statestore stores a SharedObject state as a small head and one key
// per part, so a write sets only the keys it changes instead of the whole
// state.
package sobject_statestore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"os"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Key suffixes under a Store's prefix.
const (
	headKey       = "head"
	stateKey      = "state/"
	checkpointKey = stateKey + "checkpoint"
	controlKey    = stateKey + "control/"
	epochKey      = stateKey + "epoch/"
	metaKey       = stateKey + "meta"
	opKey         = stateKey + "op/"
	seqKey        = stateKey + "seq/"
)

// LayoutEnv selects how a provider stores SharedObject states: "whole"
// selects the whole encoded state each provider stored before this package,
// and any other value, or none, this package's layout.
const LayoutEnv = "SPACEWAVE_SOBJECT_STATE_LAYOUT"

// LogSelected reports whether LayoutEnv selects the log layout. Each provider
// reads it once at startup.
func LogSelected() bool {
	return os.Getenv(LayoutEnv) != "whole"
}

// ErrOutOfOrder is returned by Write when a list of the state it is given is
// not in the order SOState keeps it. The write stores nothing rather than
// store a delta computed from unsorted lists.
var ErrOutOfOrder = errors.New("shared object state list is out of order")

// Store is the stored layout of one SharedObject state under a key prefix.
//
// Its keys, below the prefix, are:
//
//	head                        Head, which every write compares and sets
//	state/checkpoint            SOCheckpoint
//	state/control/<hash>        SOControlMessage
//	state/epoch/<epoch>         SOKeyEpoch
//	state/meta                  SOState holding only Config and Invites
//	state/op/<hash>             SOOperation
//	state/seq/<height>/<hash>   SOSequence
//
// Hashes are lower-case hex and numbers are 16 hex digits, so each list's key
// order is the order SOState keeps it in: Load decodes without sorting, and
// Write finds a delta by walking two sorted lists.
//
// Every state/ key belongs to the state the head names. Write deletes each key
// its delta drops and Replace deletes every key it does not write, because
// Load assembles whatever the keys hold, and a stray key would add an
// operation to the state.
type Store struct {
	prefix []byte
}

// New returns the Store under prefix.
func New(prefix []byte) Store {
	return Store{prefix: prefix}
}

// HeadKey returns the key of the head. A state is stored when it exists.
func (s Store) HeadKey() []byte {
	return s.key(headKey)
}

// key returns prefix followed by the parts.
func (s Store) key(parts ...string) []byte {
	// Size the key once, then fill it.
	n := len(s.prefix)
	for _, p := range parts {
		n += len(p)
	}
	k := make([]byte, 0, n)
	k = append(k, s.prefix...)
	for _, p := range parts {
		k = append(k, p...)
	}
	return k
}

// Load reads the stored state and the encoded head. found is false when no
// head is stored. It reads the head and iterates the state/ keys once, in key
// order, which is the order of each list.
func (s Store) Load(ctx context.Context, tx kvtx.TxOps) (state *sobject.SOState, head []byte, found bool, err error) {
	// Read the head; with none, no state is stored.
	head, found, err = tx.Get(ctx, s.HeadKey())
	if err != nil || !found {
		return nil, nil, found, err
	}
	head = bytes.Clone(head)

	// Decode each part into its place.
	state = &sobject.SOState{}
	prefix := s.key(stateKey)
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		name := string(it.Key()[len(s.prefix):])
		value, err := it.Value()
		if err != nil {
			return nil, nil, false, err
		}
		if err := decodePart(state, name, value); err != nil {
			return nil, nil, false, errors.Wrap(err, name)
		}
	}
	if err := it.Err(); err != nil {
		return nil, nil, false, err
	}
	return state, head, true, nil
}

// decodePart decodes the value of the state/ key name into state.
func decodePart(state *sobject.SOState, name string, value []byte) error {
	switch {
	case name == checkpointKey:
		state.Checkpoint = &sobject.SOCheckpoint{}
		return state.Checkpoint.UnmarshalVT(value)
	case name == metaKey:
		meta := &sobject.SOState{}
		if err := meta.UnmarshalVT(value); err != nil {
			return err
		}
		state.Config, state.Invites = meta.GetConfig(), meta.GetInvites()
		return nil
	case hasPrefix(name, controlKey):
		msg := &sobject.SOControlMessage{}
		state.ControlMessages = append(state.ControlMessages, msg)
		return msg.UnmarshalVT(value)
	case hasPrefix(name, epochKey):
		epoch := &sobject.SOKeyEpoch{}
		state.KeyEpochs = append(state.KeyEpochs, epoch)
		return epoch.UnmarshalVT(value)
	case hasPrefix(name, opKey):
		op := &sobject.SOOperation{}
		state.Ops = append(state.Ops, op)
		return op.UnmarshalVT(value)
	case hasPrefix(name, seqKey):
		record := &sobject.SOSequence{}
		state.Sequence = append(state.Sequence, record)
		return record.UnmarshalVT(value)
	default:
		return errors.New("unknown shared object state key")
	}
}

// hasPrefix reports whether s starts with prefix.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// Write stores next over prev, the state storage holds under the encoded head,
// and returns the encoded new head. It sets each key whose part differs and
// deletes each key prev holds and next lacks. The caller compares the stored
// head with head in the same transaction first.
//
// Lists are compared by pointer before content: SOState.Clone shares each
// signed entry, so an unchanged entry costs one pointer compare and a changed
// one a key and an encoding. Nothing else allocates or reads.
func (s Store) Write(ctx context.Context, tx kvtx.TxOps, head []byte, prev, next *sobject.SOState) ([]byte, error) {
	w, err := s.newWriter(ctx, tx, head)
	if err != nil {
		return nil, err
	}
	if err := w.diff(prev, next); err != nil {
		return nil, err
	}
	return w.finish()
}

// Replace stores state as the whole state, deleting every state/ key it does
// not write, and returns the encoded new head. A head already stored is
// extended, so the new head differs from every head a writer of the replaced
// state could hold.
func (s Store) Replace(ctx context.Context, tx kvtx.TxOps, state *sobject.SOState) ([]byte, error) {
	// Extend the stored head, if any.
	head, _, err := tx.Get(ctx, s.HeadKey())
	if err != nil {
		return nil, err
	}
	w, err := s.newWriter(ctx, tx, head)
	if err != nil {
		return nil, err
	}

	// Delete every stored part, then write the state.
	var stale [][]byte
	err = tx.ScanPrefixKeys(ctx, s.key(stateKey), func(key []byte) error {
		stale = append(stale, bytes.Clone(key))
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, key := range stale {
		if err := w.del(key); err != nil {
			return nil, err
		}
	}
	if err := w.diff(&sobject.SOState{}, state); err != nil {
		return nil, err
	}
	return w.finish()
}

// Clear deletes the head and every state/ key.
func (s Store) Clear(ctx context.Context, tx kvtx.TxOps) error {
	// Collect the keys, then delete them with the head.
	var keys [][]byte
	err := tx.ScanPrefixKeys(ctx, s.key(stateKey), func(key []byte) error {
		keys = append(keys, bytes.Clone(key))
		return nil
	})
	if err != nil {
		return err
	}
	keys = append(keys, s.HeadKey())
	for _, key := range keys {
		if err := tx.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// writer applies one write's changes and folds each into the next head's
// digest.
type writer struct {
	ctx    context.Context
	tx     kvtx.TxOps
	s      Store
	prev   *Head
	digest hash.Hash
}

// newWriter starts a write over the encoded head, which is empty when none is
// stored.
func (s Store) newWriter(ctx context.Context, tx kvtx.TxOps, head []byte) (*writer, error) {
	// Decode the previous head and seed the digest with its digest.
	prev := &Head{}
	if err := prev.UnmarshalVT(head); err != nil {
		return nil, errors.Wrap(err, "shared object state head")
	}
	digest := sha256.New()
	digest.Write(prev.GetDigest())
	return &writer{ctx: ctx, tx: tx, s: s, prev: prev, digest: digest}, nil
}

// fold adds one change to the digest: whether it sets, the key, and the value.
func (w *writer) fold(set bool, key, value []byte) {
	// Frame the change by its kind and lengths, so no two changes hash alike.
	var frame [17]byte
	if set {
		frame[0] = 1
	}
	binary.BigEndian.PutUint64(frame[1:9], uint64(len(key)))
	binary.BigEndian.PutUint64(frame[9:], uint64(len(value)))
	w.digest.Write(frame[:])
	w.digest.Write(key)
	w.digest.Write(value)
}

// set encodes msg under key.
func (w *writer) set(key []byte, msg interface{ MarshalVT() ([]byte, error) }) error {
	data, err := msg.MarshalVT()
	if err != nil {
		return err
	}
	w.fold(true, key, data)
	return w.tx.Set(w.ctx, key, data)
}

// del deletes key.
func (w *writer) del(key []byte) error {
	w.fold(false, key, nil)
	return w.tx.Delete(w.ctx, key)
}

// finish stores and returns the next head.
func (w *writer) finish() ([]byte, error) {
	// Count the write and store the head.
	next := &Head{Revision: w.prev.GetRevision() + 1, Digest: w.digest.Sum(nil)}
	data, err := next.MarshalVT()
	if err != nil {
		return nil, err
	}
	if err := w.tx.Set(w.ctx, w.s.HeadKey(), data); err != nil {
		return nil, err
	}
	return data, nil
}

// diff writes the parts of next that differ from prev.
func (w *writer) diff(prev, next *sobject.SOState) error {
	// Write the config and invites when either changed.
	if !next.GetConfig().EqualVT(prev.GetConfig()) || !equalList(prev.GetInvites(), next.GetInvites()) {
		meta := &sobject.SOState{Config: next.GetConfig(), Invites: next.GetInvites()}
		if err := w.set(w.s.key(metaKey), meta); err != nil {
			return err
		}
	}

	// Write or delete the checkpoint when it changed.
	switch prevCk, nextCk := prev.GetCheckpoint(), next.GetCheckpoint(); {
	case nextCk == nil && prevCk != nil:
		if err := w.del(w.s.key(checkpointKey)); err != nil {
			return err
		}
	case nextCk != nil && nextCk != prevCk && !nextCk.EqualVT(prevCk):
		if err := w.set(w.s.key(checkpointKey), nextCk); err != nil {
			return err
		}
	}

	// Write the changed entries of each list.
	if err := diffList(w, prev.GetKeyEpochs(), next.GetKeyEpochs(), epochName); err != nil {
		return errors.Wrap(err, "key epochs")
	}
	if err := diffList(w, prev.GetOps(), next.GetOps(), opName); err != nil {
		return errors.Wrap(err, "operations")
	}
	if err := diffList(w, prev.GetSequence(), next.GetSequence(), seqName); err != nil {
		return errors.Wrap(err, "sequence")
	}
	if err := diffList(w, prev.GetControlMessages(), next.GetControlMessages(), controlName); err != nil {
		return errors.Wrap(err, "control messages")
	}
	return nil
}

// part is a stored list entry.
type part[T any] interface {
	*T
	// MarshalVT encodes the entry.
	MarshalVT() ([]byte, error)
	// EqualVT reports whether the entry equals another.
	EqualVT(*T) bool
}

// diffList walks prev and next, two lists in key order, setting each entry
// next adds or changes and deleting each entry next drops. An entry the lists
// share by pointer is skipped without computing its key. name returns an
// entry's key below the Store's prefix.
func diffList[T any, P part[T]](w *writer, prev, next []P, name func(P) (string, error)) error {
	// The cursors, the names at each, and the last name of each list.
	var lastPrev, lastNext string
	var i, j int
	var prevName, nextName string
	var prevOK, nextOK bool
	for i < len(prev) || j < len(next) {
		// Skip a shared entry.
		if i < len(prev) && j < len(next) && prev[i] == next[j] {
			i, j = i+1, j+1
			prevOK, nextOK = false, false
			continue
		}

		// Name the entries at the cursors, each once, and check the order.
		if i < len(prev) && !prevOK {
			n, err := name(prev[i])
			if err != nil {
				return err
			}
			if n <= lastPrev && lastPrev != "" {
				return ErrOutOfOrder
			}
			prevName, lastPrev, prevOK = n, n, true
		}
		if j < len(next) && !nextOK {
			n, err := name(next[j])
			if err != nil {
				return err
			}
			if n <= lastNext && lastNext != "" {
				return ErrOutOfOrder
			}
			nextName, lastNext, nextOK = n, n, true
		}

		// Delete what next drops, set what it adds or changes.
		switch {
		case j == len(next) || i < len(prev) && prevName < nextName:
			if err := w.del(w.s.key(prevName)); err != nil {
				return err
			}
			i, prevOK = i+1, false
		case i == len(prev) || nextName < prevName:
			if err := w.set(w.s.key(nextName), next[j]); err != nil {
				return err
			}
			j, nextOK = j+1, false
		default:
			if !next[j].EqualVT((*T)(prev[i])) {
				if err := w.set(w.s.key(nextName), next[j]); err != nil {
					return err
				}
			}
			i, j = i+1, j+1
			prevOK, nextOK = false, false
		}
	}
	return nil
}

// equalList reports whether a and b hold equal entries in the same order.
func equalList[T any, P part[T]](a, b []P) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] && !a[i].EqualVT((*T)(b[i])) {
			return false
		}
	}
	return true
}

// epochName names a key epoch by its number.
func epochName(e *sobject.SOKeyEpoch) (string, error) {
	return epochKey + hex16(e.GetEpoch()), nil
}

// opName names an operation by its hash.
func opName(op *sobject.SOOperation) (string, error) {
	return opKey + hex.EncodeToString(op.Hash()), nil
}

// seqName names a position by its height, then its hash.
func seqName(record *sobject.SOSequence) (string, error) {
	height, err := record.UnmarshalHeight()
	if err != nil {
		return "", err
	}
	return seqKey + hex16(height) + "/" + hex.EncodeToString(record.Hash()), nil
}

// controlName names a control message by its hash.
func controlName(msg *sobject.SOControlMessage) (string, error) {
	return controlKey + hex.EncodeToString(msg.Hash()), nil
}

// hex16 formats n as 16 lower-case hex digits, so numbers sort as strings.
func hex16(n uint64) string {
	s := strconv.FormatUint(n, 16)
	const zeros = "0000000000000000"
	return zeros[:16-len(s)] + s
}

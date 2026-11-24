package sobject_sync

import (
	"bytes"
	"crypto/sha256"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
)

// A catch-up sends only the records the receiver lacks. The receiver's
// history request names each record of the state it holds, its config,
// checkpoint, key epochs, operations, positions and control messages, by
// record hash. The sender omits each record the receiver named and says where
// it goes back; the receiver puts its own copy there and checks the rebuilt
// state against the advertised digest. A peer k operations behind receives
// the k operations, a few runs and the state's small fields. A receiver that
// holds nothing receives every record, so a full snapshot takes the same path.
//
// A record hash is SHA-256 of the record's encoding, not its Hash: Hash covers
// only Inner, so two copies of one operation with different signatures share
// it, and putting back the wrong copy would fail the digest.

// errHeldRecord is returned when a snapshot puts back a record the receiver
// did not offer, or places one at an impossible index.
var errHeldRecord = errors.New("snapshot restores an unknown or misplaced held record")

// syncRecord is a record a SOState holds.
type syncRecord interface {
	// MarshalVT returns the record's encoding.
	MarshalVT() ([]byte, error)
}

// recordHash returns the record hash of record.
func recordHash(record syncRecord) ([sha256.Size]byte, error) {
	data, err := record.MarshalVT()
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(data), nil
}

// recordHashes returns the record hash of each record, in order.
func recordHashes[T syncRecord](records []T) ([][]byte, error) {
	hashes := make([][]byte, 0, len(records))
	for _, record := range records {
		hash, err := recordHash(record)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, hash[:])
	}
	return hashes, nil
}

// offerHeld names the records of state, the receiver's pinned state, in a
// history request. When the names would not fit in one frame it names none,
// and the sender sends every record.
func offerHeld(state *sobject.SOState, request *SOSyncHistoryRequest) error {
	// Name the singular records.
	held := &SOSyncHeld{}
	if config := state.GetConfig(); config != nil {
		hash, err := recordHash(config)
		if err != nil {
			return err
		}
		held.Config = hash[:]
	}
	if checkpoint := state.GetCheckpoint(); checkpoint != nil {
		hash, err := recordHash(checkpoint)
		if err != nil {
			return err
		}
		held.Checkpoint = hash[:]
	}

	// Name each list.
	var err error
	if held.KeyEpochs, err = recordHashes(state.GetKeyEpochs()); err != nil {
		return err
	}
	if held.Ops, err = recordHashes(state.GetOps()); err != nil {
		return err
	}
	if held.Sequence, err = recordHashes(state.GetSequence()); err != nil {
		return err
	}
	if held.ControlMessages, err = recordHashes(state.GetControlMessages()); err != nil {
		return err
	}

	// Offer nothing when the names exceed the frame budget.
	request.Held = held
	if (&SOSyncMessage{Body: &SOSyncMessage_HistoryRequest{HistoryRequest: request}}).SizeVT() > maxMessageSize {
		request.Held = nil
	}
	return nil
}

// omitHeld returns records without those the offer names, and the runs that
// put them back.
func omitHeld[T syncRecord](records []T, offer [][]byte) ([]T, []*SOSyncHeldRun, error) {
	// A receiver that holds nothing receives every record.
	if len(offer) == 0 {
		return records, nil, nil
	}
	positions := make(map[[sha256.Size]byte]uint32, len(offer))
	for i, hash := range offer {
		if len(hash) == sha256.Size {
			positions[[sha256.Size]byte(hash)] = uint32(i)
		}
	}

	// Keep each record the receiver lacks, and extend or start a run for each
	// it holds.
	kept := make([]T, 0, len(records))
	var runs []*SOSyncHeldRun
	for i, record := range records {
		hash, err := recordHash(record)
		if err != nil {
			return nil, nil, err
		}
		offered, ok := positions[hash]
		if !ok {
			kept = append(kept, record)
			continue
		}
		index := uint32(i)
		if n := len(runs); n != 0 {
			last := runs[n-1]
			if last.Index+last.Count == index && last.Offer+last.Count == offered {
				last.Count++
				continue
			}
		}
		runs = append(runs, &SOSyncHeldRun{Index: index, Offer: offered, Count: 1})
	}
	return kept, runs, nil
}

// restoreHeld rebuilds a list from the delivered records and the runs, which
// place records of held, the receiver's pinned list, in increasing index
// order. Every delivered record and every placed record lands at exactly one
// index.
func restoreHeld[T syncRecord](delivered []T, runs []*SOSyncHeldRun, held []T) ([]T, error) {
	// A snapshot that omits nothing is complete.
	if len(runs) == 0 {
		return delivered, nil
	}

	// Fill the gap before each run from the delivered records, then the run
	// from the held list.
	var out []T
	for _, run := range runs {
		index, offer, count := int(run.GetIndex()), int(run.GetOffer()), int(run.GetCount())
		gap := index - len(out)
		if count == 0 || gap < 0 || gap > len(delivered) || offer+count > len(held) {
			return nil, errHeldRecord
		}
		out = append(out, delivered[:gap]...)
		delivered = delivered[gap:]
		out = append(out, held[offer:offer+count]...)
	}
	return append(out, delivered...), nil
}

// omitRequestHeld removes the records the request offered from state, a
// clone the caller owns, and records in snapshot where they go back.
func omitRequestHeld(state *sobject.SOState, request *SOSyncHistoryRequest, snapshot *SOSyncSnapshot) error {
	// Omit the singular records the receiver holds byte for byte.
	held := request.GetHeld()
	if held == nil {
		return nil
	}
	restore := &SOSyncRestore{}
	if config := state.GetConfig(); config != nil && len(held.GetConfig()) != 0 {
		hash, err := recordHash(config)
		if err != nil {
			return err
		}
		if restore.Config = bytes.Equal(hash[:], held.GetConfig()); restore.Config {
			state.Config = nil
		}
	}
	if checkpoint := state.GetCheckpoint(); checkpoint != nil && len(held.GetCheckpoint()) != 0 {
		hash, err := recordHash(checkpoint)
		if err != nil {
			return err
		}
		if restore.Checkpoint = bytes.Equal(hash[:], held.GetCheckpoint()); restore.Checkpoint {
			state.Checkpoint = nil
		}
	}

	// Omit from each list in turn.
	var err error
	if state.KeyEpochs, restore.KeyEpochs, err = omitHeld(state.KeyEpochs, held.GetKeyEpochs()); err != nil {
		return err
	}
	if state.Ops, restore.Ops, err = omitHeld(state.Ops, held.GetOps()); err != nil {
		return err
	}
	if state.Sequence, restore.Sequence, err = omitHeld(state.Sequence, held.GetSequence()); err != nil {
		return err
	}
	if state.ControlMessages, restore.ControlMessages, err = omitHeld(state.ControlMessages, held.GetControlMessages()); err != nil {
		return err
	}
	snapshot.Restore = restore
	return nil
}

// restoreRequestHeld puts the records snapshot omits back into state from
// held, the state the request offered, or nil when it offered nothing.
func restoreRequestHeld(state *sobject.SOState, held *sobject.SOState, snapshot *SOSyncSnapshot) error {
	// Put back the singular records, which the snapshot must not also carry,
	// as copies the state owns.
	restore := snapshot.GetRestore()
	if restore == nil {
		return nil
	}
	if restore.GetConfig() {
		if state.Config != nil || held.GetConfig() == nil {
			return errHeldRecord
		}
		state.Config = held.GetConfig().CloneVT()
	}
	if restore.GetCheckpoint() {
		if state.Checkpoint != nil || held.GetCheckpoint() == nil {
			return errHeldRecord
		}
		state.Checkpoint = held.GetCheckpoint().CloneVT()
	}

	// Restore each list in turn. A state owns its config, checkpoint and key
	// epochs, as Clone copies them, and shares its signed records.
	var err error
	if state.KeyEpochs, err = restoreHeld(state.KeyEpochs, restore.GetKeyEpochs(), held.GetKeyEpochs()); err != nil {
		return err
	}
	for i, epoch := range state.KeyEpochs {
		state.KeyEpochs[i] = epoch.CloneVT()
	}
	if state.Ops, err = restoreHeld(state.Ops, restore.GetOps(), held.GetOps()); err != nil {
		return err
	}
	if state.Sequence, err = restoreHeld(state.Sequence, restore.GetSequence(), held.GetSequence()); err != nil {
		return err
	}
	state.ControlMessages, err = restoreHeld(state.ControlMessages, restore.GetControlMessages(), held.GetControlMessages())
	return err
}

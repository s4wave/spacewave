//go:build !tinygo

package block_store_s3

import (
	"context"
	"encoding/binary"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// PackState is a pack's sweep state, as PackSweep.tla names it.
type PackState uint8

// The sweep states of a pack.
const (
	// PackRegistered is a pack every read, locate and listing sees, the state
	// of a pack with no q/ row.
	PackRegistered PackState = iota
	// PackQuarantined is a pack held out of the reads, read only by the
	// fallback, which lifts it.
	PackQuarantined
	// PackTrashing is a quarantined pack the command is moving to the trash:
	// the fallback still reads it, and records the read as a miss.
	PackTrashing
)

// held reports whether the state holds the pack out of the reads.
func (s PackState) held() bool {
	return s == PackQuarantined || s == PackTrashing
}

// String names the state.
func (s PackState) String() string {
	switch s {
	case PackRegistered:
		return "registered"
	case PackQuarantined:
		return "quarantined"
	case PackTrashing:
		return "trashing"
	default:
		return "unknown"
	}
}

// packHold is the q/ row of a pack: its state, when it entered it, and the
// last fallback read that lifted it or, while it is trashing, missed it.
type packHold struct {
	// state is the pack's state.
	state PackState
	// since is when the pack entered state, in Unix nanoseconds.
	since int64
	// event is when the last lift or miss read the pack, in Unix nanoseconds,
	// or zero when none did. A lift by the command records no key.
	event int64
	// key is the block key the event read.
	key []byte
}

// holdHeader is the bytes of a q/ row before its key: the state and two
// times.
const holdHeader = 17

// marshal encodes the row: the state, the two times, then the key.
func (h packHold) marshal() []byte {
	// Append the fields after the state byte.
	b := make([]byte, 1, holdHeader+len(h.key))
	b[0] = byte(h.state)
	b = binary.BigEndian.AppendUint64(b, uint64(h.since)) //nolint:gosec // A time, not a size.
	b = binary.BigEndian.AppendUint64(b, uint64(h.event)) //nolint:gosec // A time, not a size.
	return append(b, h.key...)
}

func unmarshalPackHold(b []byte) (packHold, error) {
	if len(b) < holdHeader || PackState(b[0]) > PackTrashing {
		return packHold{}, errors.Errorf("pack hold row of %d bytes", len(b))
	}
	return packHold{
		state: PackState(b[0]),
		since: int64(binary.BigEndian.Uint64(b[1:])), //nolint:gosec // A time, not a size.
		event: int64(binary.BigEndian.Uint64(b[9:])), //nolint:gosec // A time, not a size.
		key:   append([]byte(nil), b[holdHeader:]...),
	}, nil
}

// readHeld reads the state of every held pack from the q/ rows.
func readHeld(ctx context.Context, tx kvtx.Tx) (map[string]PackState, error) {
	held := make(map[string]PackState)
	err := tx.ScanPrefix(ctx, []byte(indexHoldPrefix), func(key, value []byte) error {
		hold, err := unmarshalPackHold(value)
		if err != nil {
			return err
		}
		if hold.state.held() {
			held[string(key[len(indexHoldPrefix):])] = hold.state
		}
		return nil
	})
	return held, errors.Wrap(err, "read pack holds")
}

// writeHolds writes the q/ rows of ids in one transaction.
func (x *packIndex) writeHolds(ctx context.Context, ids []string, holds []packHold) error {
	// Open the write transaction.
	tx, err := x.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Set each row and commit them together.
	for i, id := range ids {
		if err := tx.Set(ctx, []byte(indexHoldPrefix+id), holds[i].marshal()); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// readHold reads the q/ row of pack id, and false when it has none.
func (x *packIndex) readHold(ctx context.Context, id string) (packHold, bool, error) {
	// Open a read transaction.
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return packHold{}, false, err
	}
	defer tx.Discard()

	// Read and decode the row.
	value, ok, err := tx.Get(ctx, []byte(indexHoldPrefix+id))
	if err != nil || !ok {
		return packHold{}, false, err
	}
	hold, err := unmarshalPackHold(value)
	return hold, err == nil, err
}

// setHeldLocked records the state of pack id in memory. Call with mtx held.
func (x *packIndex) setHeldLocked(id string, state PackState) {
	if state.held() {
		x.held[id] = state
	} else {
		delete(x.held, id)
	}
	x.heldCount.Store(int64(len(x.held)))
}

// heldLocked reports whether pack id is held. Call with mtx held.
func (x *packIndex) heldLocked(id string) bool {
	if len(x.held) == 0 || id == "" {
		return false
	}
	_, ok := x.held[id]
	return ok
}

// isHeld reports whether pack id is held.
func (x *packIndex) isHeld(id string) bool {
	if x.heldCount.Load() == 0 {
		return false
	}
	x.mtx.RLock()
	defer x.mtx.RUnlock()
	return x.heldLocked(id)
}

// heldState returns the state of pack id, and false when the table lacks it.
func (x *packIndex) heldState(id string) (PackState, bool) {
	x.mtx.RLock()
	defer x.mtx.RUnlock()
	if _, ok := x.rows[id]; !ok {
		return PackRegistered, false
	}
	return x.held[id], true
}

// heldPack returns the held pack the key's h row names, or "" when the row
// names a registered or removed pack or there is no row.
func (x *packIndex) heldPack(ctx context.Context, key []byte) (string, error) {
	id, _, err := x.locateAny(ctx, key)
	if err != nil || !x.isHeld(id) {
		return "", err
	}
	return id, nil
}

// heldLocator is the locator of the held manifest: every row, held or not,
// so a row naming a registered or removed pack sends the held lookup to every
// held filter rather than ending it.
type heldLocator struct {
	x *packIndex
}

// Locate returns the pack that holds a block key.
func (l heldLocator) Locate(ctx context.Context, key []byte) (string, bool, error) {
	return l.x.locateAny(ctx, key)
}

// Indexed reports whether every key of the pack is in h rows.
func (l heldLocator) Indexed(id string) bool {
	return l.x.Indexed(id)
}

// Version changes whenever Indexed may answer differently for some pack.
func (l heldLocator) Version() uint64 {
	return l.x.Version()
}

// errNoSweep refuses a sweep on a store it cannot hold packs in.
var errNoSweep = errors.New("the pack sweep needs a store that keeps its packs and has an index")

// Quarantine holds the registered packs ids out of every read, locate and
// listing: a read that misses every registered pack falls back to them and
// lifts the one that serves it. It writes their q/ rows in one transaction
// before any read stops seeing them. A pack already quarantined stays as it
// is, so a repeated call changes nothing; a trashing pack or one the table
// lacks fails the call before any row is written.
func (s *PackStore) Quarantine(ctx context.Context, ids []string) error {
	// Refuse a store whose own passes may rewrite a held pack.
	if s.index == nil || !s.keepPacks {
		return errNoSweep
	}
	if err := s.loadEntries(ctx); err != nil {
		return err
	}
	release, err := s.holdMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Choose the registered packs, refusing any pack past quarantine.
	var move []string
	for _, id := range ids {
		state, ok := s.index.heldState(id)
		switch {
		case !ok:
			return errors.Errorf("quarantine pack %s: the index holds no such pack", id)
		case state == PackTrashing:
			return errors.Errorf("quarantine pack %s: it is trashing", id)
		case state == PackRegistered && !slices.Contains(move, id):
			move = append(move, id)
		}
	}
	if len(move) == 0 {
		return nil
	}

	// Write their rows, then hold them out of the reads.
	now := time.Now().UnixNano()
	holds := make([]packHold, len(move))
	for i := range holds {
		holds[i] = packHold{state: PackQuarantined, since: now}
	}
	if err := s.index.writeHolds(ctx, move, holds); err != nil {
		return err
	}
	s.setHolds(move, PackQuarantined)
	s.le.WithField("packs", len(move)).Info("quarantined packs")
	return nil
}

// Lift returns the quarantined packs ids to the registered state, recording
// the lift without a block, as the command's own lift. A registered pack
// stays as it is; a trashing pack fails the call, since only a restore
// returns it.
func (s *PackStore) Lift(ctx context.Context, ids []string) error {
	// Lift nothing on a store that holds nothing.
	if s.index == nil {
		return errNoSweep
	}
	if err := s.loadEntries(ctx); err != nil {
		return err
	}
	release, err := s.holdMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Choose the quarantined packs.
	var move []string
	for _, id := range ids {
		state, ok := s.index.heldState(id)
		switch {
		case !ok:
			return errors.Errorf("lift pack %s: the index holds no such pack", id)
		case state == PackTrashing:
			return errors.Errorf("lift pack %s: it is trashing", id)
		case state == PackQuarantined && !slices.Contains(move, id):
			move = append(move, id)
		}
	}
	if len(move) == 0 {
		return nil
	}

	// Record the lifts, then return the packs to the reads.
	now := time.Now().UnixNano()
	holds := make([]packHold, len(move))
	for i := range holds {
		holds[i] = packHold{state: PackRegistered, since: now, event: now}
	}
	if err := s.index.writeHolds(ctx, move, holds); err != nil {
		return err
	}
	s.setHolds(move, PackRegistered)
	return nil
}

// noteFallback records that a fallback read block ref from pack id: a
// quarantined pack is lifted, and a trashing pack records the miss, each in
// the one transaction that writes its row. The read returns the block only
// after it, so no block a reader needed is served from a pack that stays
// removable.
func (s *PackStore) noteFallback(ctx context.Context, id string, ref *block.BlockRef) error {
	// Serialize with the other moves of the pack's state.
	release, err := s.holdMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	state, _ := s.index.heldState(id)
	if !state.held() {
		return nil
	}

	// Record the read, and lift a quarantined pack.
	now := time.Now().UnixNano()
	key := packfile.BlockKey(ref.GetHash())
	hold := packHold{state: PackRegistered, since: now, event: now, key: key}
	if state == PackTrashing {
		prev, _, err := s.index.readHold(ctx, id)
		if err != nil {
			return err
		}
		hold = packHold{state: PackTrashing, since: prev.since, event: now, key: key}
	}
	if err := s.index.writeHolds(ctx, []string{id}, []packHold{hold}); err != nil {
		return errors.Wrap(err, "record the fallback read of pack "+id)
	}

	// Log the miss, or publish the lift in memory and log it.
	le := s.le.WithFields(logrus.Fields{"pack": id, "block": ref.MarshalString()})
	if state == PackTrashing {
		le.Warn("a read missed a trashing pack")
		return nil
	}
	s.setHolds([]string{id}, PackRegistered)
	le.Info("lifted a quarantined pack on a read")
	return nil
}

// setHolds moves packs ids to state in memory: under bcast, the published
// entries move between the main and the held manifest with the index's state,
// adding to the destination before dropping from the source, so a concurrent
// read finds each pack in one of them. Call with holdMtx held, after the rows
// are written.
func (s *PackStore) setHolds(ids []string, state PackState) {
	s.holdMoves.Add(1)
	defer s.holdMoves.Add(1)
	s.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		// Choose the source and destination of the move.
		from, to := s.entries, s.heldEntries
		fromStore, toStore := s.packs, s.held
		if !state.held() {
			from, to = to, from
			fromStore, toStore = toStore, fromStore
		}

		// Add the published entries to the destination.
		var moved []*packfile.PackfileEntry
		var movedIDs []string
		for _, id := range ids {
			if entry := from[id]; entry != nil {
				moved = append(moved, entry)
				movedIDs = append(movedIDs, id)
				to[id] = entry
			}
		}
		toStore.ApplyManifestDelta(moved, nil)

		// Flip the index's state, then drop the entries from the source.
		s.index.mtx.Lock()
		for _, id := range ids {
			if _, ok := s.index.rows[id]; ok {
				s.index.setHeldLocked(id, state)
			}
		}
		s.index.mtx.Unlock()
		for _, id := range movedIDs {
			delete(from, id)
		}
		fromStore.ApplyManifestDelta(nil, movedIDs)
	})
}

// movedSince reports whether a manifest move ran at any point since
// holdMoves read moves.
func (s *PackStore) movedSince(moves uint64) bool {
	return moves&1 == 1 || s.holdMoves.Load() != moves
}

// publishHeld publishes the held pack the index places ref in, reading its
// entry from the index, and reports whether any pack is held.
func (s *PackStore) publishHeld(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Find the held pack the key's row names.
	if s.index == nil || s.index.heldCount.Load() == 0 || ref.GetHash() == nil {
		return false, nil
	}
	id, err := s.index.heldPack(ctx, packfile.BlockKey(ref.GetHash()))
	if err != nil || id == "" {
		return true, err
	}

	// Read and publish its entry unless it is published.
	var published bool
	s.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		published = s.heldEntries[id] != nil
	})
	if published {
		return true, nil
	}
	entries, err := s.index.readEntries(ctx, []string{id})
	if err != nil {
		return true, err
	}
	s.updateEntries(entries, nil)
	return true, nil
}

// readHeld reads a block every registered pack lacks from the held packs,
// lifting the quarantined pack that serves it, or nil when none holds it.
func (s *PackStore) readHeld(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Publish the held pack the index places the block in.
	holding, err := s.publishHeld(ctx, ref)
	if err != nil || !holding {
		return nil, err
	}

	// Read it, and record the read before returning the block.
	stored, id, err := s.held.GetStoredBlockPack(ctx, ref)
	if err != nil || stored == nil {
		return nil, err
	}
	if err := s.noteFallback(ctx, id, ref); err != nil {
		return nil, err
	}
	return stored, nil
}

// statHeld is readHeld for StatBlock.
func (s *PackStore) statHeld(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	// Publish the held pack the index places the block in.
	holding, err := s.publishHeld(ctx, ref)
	if err != nil || !holding {
		return nil, err
	}

	// Stat it, and record the read before answering.
	stat, id, err := s.held.StatBlockPack(ctx, ref)
	if err != nil || stat == nil {
		return nil, err
	}
	if err := s.noteFallback(ctx, id, ref); err != nil {
		return nil, err
	}
	return stat, nil
}

package packsweep

import (
	"bytes"
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Bounds of a removal, variables so a test can lower them.
var (
	// removeRoundKeys is the keys past which a removal stops adding packs to
	// a round. A round holds its keys and their holders in memory, and reads
	// every member row of the catalog once to find the holders.
	removeRoundKeys = 1 << 20
	// removeCallWeight is the keys and holders one backend call carries.
	removeCallWeight = MaxBackendKeys
)

// RemoveOptions configures Remove.
type RemoveOptions struct {
	// Store is the scratch store, holding the snapshot's catalog.
	Store kvtx.Store
	// Backend is the backend's side of the cycle.
	Backend Backend
	// Now returns the removal time; nil uses time.Now.
	Now func() time.Time
}

// RemoveResult reports a removal.
type RemoveResult struct {
	// Removed is the packs the backend removed, in this run or one killed
	// before it recorded them.
	Removed uint64
	// Missed is the moved packs a read missed, or that left the trashing
	// state, left for restore.
	Missed uint64
	// Rounds is the reads of the catalog's member rows the holders took.
	Rounds uint64
}

// removePack is a moved pack to remove, with its keys and their holders.
type removePack struct {
	// rec is the pack's record.
	rec *PackRecord
	// keys are the keys the catalog lists in the pack, by tail.
	keys []KeyHolders
}

// Remove has the backend remove each moved pack no read missed. It sends
// every key the snapshot lists in the pack with the other packs listing it,
// leaving out the packs the sweep moved to the trash, so the backend points
// each h row that names the pack at a holder it still reads, or deletes it.
// The packs go in rounds of about removeRoundKeys keys, each round reading the
// catalog's member rows once, and each backend call carries at most
// MaxBackendKeys keys and holders; a pack holding more sends its first keys in
// calls that repair the rows only. A pack is recorded removed with the time
// once the backend answers it absent; a run killed before that finds it
// absent and records it.
func Remove(ctx context.Context, opts *RemoveOptions) (*RemoveResult, error) {
	// Take the moved packs, and every pack whose file is in the trash.
	records, err := readPackRecords(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	trashed := make(map[string]bool)
	var moved []*PackRecord
	for _, p := range records {
		if p.Stage >= StageMoved {
			trashed[p.ID] = true
		}
		if p.Stage == StageMoved {
			moved = append(moved, p)
		}
	}

	// Read the moved packs' states.
	res := &RemoveResult{}
	if len(moved) == 0 {
		return res, nil
	}
	status, err := backendStatus(ctx, opts.Backend, moved)
	if err != nil {
		return nil, err
	}

	// Record the absent packs removed, and leave the missed for restore.
	var remove, absent []*PackRecord
	for i, p := range moved {
		switch h := status.Packs[i]; {
		case h.Absent:
			absent = append(absent, p)
		case h.State == PackTrashing && h.Event.IsZero():
			remove = append(remove, p)
		default:
			res.Missed++
		}
	}
	if err := recordRemoved(ctx, opts, absent, res); err != nil {
		return nil, err
	}

	// Remove a round of packs at a time.
	for len(remove) != 0 {
		var round []*removePack
		if round, remove, err = takeRound(ctx, opts.Store, remove); err != nil {
			return nil, err
		}
		if err := findHolders(ctx, opts.Store, round, trashed); err != nil {
			return nil, err
		}
		res.Rounds++
		if err := removeRound(ctx, opts, round, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// recordRemoved records the packs removed at the options' time.
func recordRemoved(ctx context.Context, opts *RemoveOptions, packs []*PackRecord, res *RemoveResult) error {
	// Read the clock once for the batch.
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	at := now().UTC()

	// Record each pack removed.
	for _, p := range packs {
		p.Stage, p.Removed = StageRemoved, at
	}
	res.Removed += uint64(len(packs))
	return writePackRecords(ctx, opts.Store, packs, nil)
}

// takeRound reads the keys of the first packs of todo, until they hold
// removeRoundKeys keys, and returns those packs and the rest. A round takes
// at least one pack.
func takeRound(ctx context.Context, store kvtx.Store, todo []*PackRecord) ([]*removePack, []*PackRecord, error) {
	// Read the packs' member rows in one read.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Discard()

	// Take packs until the round is full.
	var round []*removePack
	keys := 0
	for len(todo) != 0 && (len(round) == 0 || keys < removeRoundKeys) {
		p := &removePack{rec: todo[0]}
		prefix := []byte(catalogMemberPrefix + p.rec.ID + "/")
		err := tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
			p.keys = append(p.keys, KeyHolders{Key: bytes.Clone(key[len(prefix):])})
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
		round, todo, keys = append(round, p), todo[1:], keys+len(p.keys)
	}
	return round, todo, nil
}

// findHolders reads every member row of the catalog once and adds to each
// key of the round the other packs that hold it, up to MaxKeyHolders, leaving
// out the packs in trashed, whose files are in the trash.
func findHolders(ctx context.Context, store kvtx.Store, round []*removePack, trashed map[string]bool) error {
	// Index the round's keys by tail.
	index := make(map[string]*KeyHolders)
	for _, p := range round {
		for i := range p.keys {
			index[string(p.keys[i].Key)] = &p.keys[i]
		}
	}

	// Read the member rows, each pack id then its key's tail.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return tx.ScanPrefixKeys(ctx, []byte(catalogMemberPrefix), func(key []byte) error {
		// Split the row's key, and skip a pack whose file is in the trash.
		rest := key[len(catalogMemberPrefix):]
		slash := bytes.IndexByte(rest, '/')
		if slash < 0 {
			return errors.Errorf("malformed catalog member key %q", key)
		}
		id, tail := rest[:slash], rest[slash+1:]
		if trashed[string(id)] {
			return nil
		}

		// Add the pack to the key's holders.
		k := index[string(tail)]
		if k != nil && len(k.Others) < MaxKeyHolders {
			k.Others = append(k.Others, string(id))
		}
		return nil
	})
}

// removeRound has the backend remove the round's packs, as many in a call as
// MaxBackendPacks and MaxBackendKeys allow.
func removeRound(ctx context.Context, opts *RemoveOptions, round []*removePack, res *RemoveResult) error {
	// Fill each call, sending it before a pack would overflow it.
	var call removeCall
	for _, p := range round {
		w := keysWeight(p.keys)
		if len(call.packs) != 0 && (call.weight+w > removeCallWeight || len(call.packs) == MaxBackendPacks) {
			if err := call.send(ctx, opts, res); err != nil {
				return err
			}
			call = removeCall{}
		}

		// A pack holding more than one call carries repairs its first keys
		// alone, and the rest go with the next call.
		keys := p.keys
		for keysWeight(keys) > removeCallWeight {
			n := keysFitting(keys, removeCallWeight)
			if _, err := opts.Backend.Remove(ctx, []string{p.rec.ID}, keys[:n], true); err != nil {
				return errors.Wrap(err, "repair the rows of pack "+p.rec.ID)
			}
			keys = keys[n:]
		}
		call.packs = append(call.packs, p.rec)
		call.keys = append(call.keys, keys...)
		call.weight += keysWeight(keys)
	}
	if len(call.packs) == 0 {
		return nil
	}
	return call.send(ctx, opts, res)
}

// removeCall is one backend call removing packs.
type removeCall struct {
	// packs are the packs the call removes.
	packs []*PackRecord
	// keys are the keys it repairs.
	keys []KeyHolders
	// weight is the keys and holders together.
	weight int
}

// send removes the call's packs, checks the backend answers each absent, and
// records them removed.
func (c *removeCall) send(ctx context.Context, opts *RemoveOptions, res *RemoveResult) error {
	// Remove the packs.
	ids := make([]string, len(c.packs))
	for i, p := range c.packs {
		ids[i] = p.ID
	}
	holds, err := opts.Backend.Remove(ctx, ids, c.keys, false)
	if err != nil {
		return errors.Wrap(err, "remove")
	}

	// Check each is gone, then record them.
	if len(holds) != len(ids) {
		return errors.Errorf("the backend answered %d of %d packs", len(holds), len(ids))
	}
	for i, h := range holds {
		if h.ID != ids[i] || !h.Absent {
			return errors.Errorf("pack %s is in state %d after its removal", ids[i], h.State)
		}
	}
	return recordRemoved(ctx, opts, c.packs, res)
}

// keysWeight returns the keys and their holders together.
func keysWeight(keys []KeyHolders) int {
	w := 0
	for _, k := range keys {
		w += 1 + len(k.Others)
	}
	return w
}

// keysFitting returns how many of the first keys fit in weight limit, at
// least one.
func keysFitting(keys []KeyHolders, limit int) int {
	w := 0
	for i, k := range keys {
		w += 1 + len(k.Others)
		if w > limit {
			return max(i, 1)
		}
	}
	return len(keys)
}

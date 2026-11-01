package packmove

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// Switch copies cfg.Source into the empty cfg.Dest at the current format,
// leaving out the value of every World block the pack log committed. The copy
// keeps every other key, converts the reference graph as a volume copy does,
// and walks the block range by key alone: no packed block's value is read.
//
// It keeps the value of every block outside the World. A World block the log
// did not commit, because it was written after Pack passed its key or in a
// pack that never committed, is copied with its value and a fresh upload
// marker under cfg.WritebackPrefix, so the write-back uploader sends it to the
// bucket later; a block the log recorded as bad is copied without one. The
// source's own records under the prefix are left out. Source is never written.
func Switch(ctx context.Context, cfg SwitchConfig) (SwitchResult, error) {
	// Refuse a prefix that would leave out every key.
	if len(cfg.WritebackPrefix) == 0 {
		return SwitchResult{}, errors.New("switch needs the write-back object store prefix")
	}

	// Find the blocks that stay, with the source closed by other processes.
	outside, err := FindOutsideWorld(ctx, cfg.Source, cfg.Keys, cfg.WorldStoreID)
	if err != nil {
		return SwitchResult{}, errors.Wrap(err, "find the blocks outside the World")
	}
	if cfg.SkipBlocks && outside.Len() != 0 {
		return SwitchResult{}, errors.Errorf("skip blocks would leave out %d blocks outside the World", outside.Len())
	}

	// Read the logs' entries in key order alongside the block range.
	cursor, err := openPackedCursor(cfg.LogPaths)
	if err != nil {
		return SwitchResult{}, errors.Wrap(err, "open pack log")
	}
	defer func() { _ = cursor.close() }()

	// Leave out the packed World blocks, remembering what the log said about
	// the others for the keys they get.
	result := SwitchResult{Outside: outside.Len(), OutsideBytes: outside.Bytes()}
	states := make(map[string]keyState)
	packed := func(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
		clear(states)
		drop := make([]bool, len(refs))
		for i, ref := range refs {
			key, err := ref.MarshalVT()
			if err != nil {
				return nil, err
			}
			if outside.Has(key) {
				continue
			}
			state, err := cursor.state(key)
			if err != nil {
				return nil, err
			}
			states[string(key)] = state
			drop[i] = state == keyPacked
		}
		return drop, nil
	}

	// Queue the World blocks no pack holds, and none that is bad.
	kept := func(_ context.Context, _ kvtx.Tx, ref *block.BlockRef, value []byte) ([]volume_kvtx.CopyKey, error) {
		// Look up what the log said about the block.
		key, err := ref.MarshalVT()
		if err != nil {
			return nil, err
		}
		state, world := states[string(key)]
		switch {
		case !world:
			return nil, nil
		case state == keyBad:
			result.Bad++
			return nil, nil
		}
		result.Unpacked++
		result.UnpackedBytes += uint64(len(value))
		return markerKeys(cfg.WritebackPrefix, ref, value), nil
	}

	// Copy.
	omit := [][]byte{cfg.WritebackPrefix}
	if cfg.OmitGraph {
		omit = append(omit, volume_kvtx.RefGraphPrefixes()...)
	}
	if cfg.SkipBlocks {
		omit = append(omit, cfg.Keys.GetBlockFullPrefix())
	}
	opts := volume_kvtx.SplitOptions{Drop: packed, Kept: kept, Omit: omit}
	err = volume_kvtx.CopyStoreSplitting(ctx, cfg.Source, cfg.Dest, cfg.Keys, opts, func(p volume_kvtx.CopyProgress) {
		result.Copy = p
		if cfg.Progress != nil {
			cfg.Progress(p)
		}
	})
	return result, err
}

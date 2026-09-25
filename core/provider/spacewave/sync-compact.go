package provider_spacewave

import (
	"bytes"
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

const (
	// compactMinPacks is the number of small packs that starts a merge.
	compactMinPacks = 16
	// compactMaxPacks is the server's limit on packs replaced by one push.
	compactMaxPacks = 32
	// compactSmallPackBytes marks a pack as small. Any compactMinPacks small
	// packs fit one merged pack under the sync pack target.
	compactSmallPackBytes = syncFlushMaxPackBytes / compactMinPacks
)

// CompactNow merges the oldest small committed packs into one replacement pack
// when at least compactMinPacks have accumulated. It holds the flush lock, so a
// merge never races a flush. A concurrent pull that replaces an input makes the
// merge push fail with a replacement conflict.
//
// A failed merge holds further merges until the next pull refreshes the
// manifest. A replacement conflict pulls immediately.
func (s *syncController) CompactNow(ctx context.Context) error {
	s.flushMtx.Lock()
	defer s.flushMtx.Unlock()
	if s.compactWaitPull.Load() {
		return nil
	}
	inputs := planCompaction(s.mfst.GetEntries())
	if len(inputs) == 0 {
		return nil
	}

	err := s.mergePacks(ctx, inputs)
	if err == nil || ctx.Err() != nil {
		return err
	}
	s.compactWaitPull.Store(true)
	if clouderror.IsPackReplacementConflict(err) {
		s.le.WithError(err).Debug("small pack merge lost a replacement race, pulling")
		return s.PullNow(ctx)
	}
	return err
}

// planCompaction picks the oldest small committed packs for one merge, up to
// compactMaxPacks and the sync pack target. It returns nil when fewer than
// compactMinPacks qualify. Packs without a sequence have not been pulled back
// from the server and cannot be replaced yet.
func planCompaction(entries []*packfile.PackfileEntry) []*packfile.PackfileEntry {
	var small []*packfile.PackfileEntry
	for _, entry := range entries {
		if entry.GetSequence() == 0 || entry.GetSupersededBy() != "" {
			continue
		}
		if int64(entry.GetSizeBytes()) >= compactSmallPackBytes { //nolint:gosec // pack sizes are bounded by writer.DefaultMaxPackBytes.
			continue
		}
		small = append(small, entry)
	}
	if len(small) < compactMinPacks {
		return nil
	}
	slices.SortFunc(small, func(a, b *packfile.PackfileEntry) int {
		return cmp.Compare(a.GetSequence(), b.GetSequence())
	})

	var total int64
	n := 0
	for n < len(small) && n < compactMaxPacks {
		size := int64(small[n].GetSizeBytes()) //nolint:gosec // bounded by compactSmallPackBytes above.
		if total+size > syncFlushMaxPackBytes {
			break
		}
		total += size
		n++
	}
	return small[:n]
}

// mergePacks reads every input pack, writes their blocks into one pack, pushes
// it as a replacement for the inputs, and commits the replacement locally.
func (s *syncController) mergePacks(ctx context.Context, inputs []*packfile.PackfileEntry) error {
	started := time.Now()
	chunk, err := s.prepareMergedPack(ctx, inputs)
	if err != nil {
		return err
	}
	if err := s.pushPreparedChunk(ctx, chunk); err != nil {
		return err
	}

	event := &packfile.PackReplacementEvent{ReplacedPackIds: chunk.replaces}
	if err := s.applyManifestDelta(ctx, []*packfile.PackfileEntry{chunk.entry}, []*packfile.PackReplacementEvent{event}); err != nil {
		return errors.Wrap(err, "applying merge delta")
	}
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.addSyncTelemetryMerge(id, len(chunk.replaces))
	})

	s.le.WithField("pack-id", chunk.entry.GetId()).
		WithField("replaced", len(chunk.replaces)).
		WithField("blocks", chunk.entry.GetBlockCount()).
		WithField("bytes", len(chunk.packData)).
		WithField("duration", time.Since(started)).
		Debug("merged small packs")
	return nil
}

// prepareMergedPack builds one pack holding every block of the inputs. Blocks
// keep their stored value order, inputs oldest first. The merged key set is
// checked against the union of the input key sets before the pack is returned,
// so a replacement can never drop a block.
func (s *syncController) prepareMergedPack(ctx context.Context, inputs []*packfile.PackfileEntry) (*preparedSyncChunk, error) {
	replaces := make([]string, len(inputs))
	seen := make(map[string]struct{})
	var blocks []packfile_store.PackBlock
	for i, input := range inputs {
		replaces[i] = input.GetId()
		read, err := s.lower.ReadPackBlocks(ctx, input.GetId(), int64(input.GetSizeBytes())) //nolint:gosec // pack sizes are bounded by writer.DefaultMaxPackBytes.
		if err != nil {
			return nil, errors.Wrapf(err, "reading pack %s", input.GetId())
		}
		for _, b := range read {
			key := b.Hash.MarshalString()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			blocks = append(blocks, b)
		}
	}

	var buf bytes.Buffer
	idx := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		b := blocks[idx]
		idx++
		return b.Hash, b.Block, nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "packing merged blocks")
	}
	if err := checkPackKeys(buf.Bytes(), seen); err != nil {
		return nil, err
	}
	packID, err := identity.BuildPackID(s.resourceID, result)
	if err != nil {
		return nil, errors.Wrap(err, "build merged pack id")
	}

	return &preparedSyncChunk{
		replaces: replaces,
		entry: &packfile.PackfileEntry{
			Id:                 packID,
			BloomFilter:        result.BloomFilter,
			BloomFormatVersion: packfile.BloomFormatVersionV1,
			BlockCount:         result.BlockCount,
			SizeBytes:          result.BytesWritten,
			CreatedAt:          timestamppb.New(time.Now().UTC()),
		},
		packData: buf.Bytes(),
		bodyHash: result.PackBytesDigest,
	}, nil
}

// checkPackKeys verifies the written pack indexes exactly the want key set.
func checkPackKeys(packData []byte, want map[string]struct{}) error {
	rd, err := kvfile.BuildReader(bytes.NewReader(packData), uint64(len(packData)))
	if err != nil {
		return errors.Wrap(err, "read merged pack index")
	}
	var count int
	err = rd.ScanPrefixKeys(nil, func(key []byte) error {
		if _, ok := want[string(key)]; !ok {
			return errors.Errorf("merged pack holds unexpected key %s", key)
		}
		count++
		return nil
	})
	if err != nil {
		return err
	}
	if count != len(want) {
		return errors.Errorf("merged pack holds %d of %d input blocks", count, len(want))
	}
	return nil
}

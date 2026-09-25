package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"strconv"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

const (
	// maxPackBytes bounds every ingest and complete relocation operation.
	maxPackBytes = 4 << 20
	// maxPackRecords bounds a pack's directory and location updates.
	maxPackRecords = 512
	// blockPrefix separates full block identities from other ordered records.
	blockPrefix = "\x00b"
	// packPrefix indexes bounded pack directories and live-byte accounting.
	packPrefix = "\x00p"
	// cleanupPrefix orders dirty packs by their first deletion generation.
	cleanupPrefix = "\x00q"
)

// packStore implements durable block operations with external immutable payloads.
type packStore struct {
	// engine owns publication and reader protection.
	engine *Engine
	// hashType selects content identities when callers do not specify one.
	hashType hash.HashType
	// read is an optional pinned operation scope released by its caller.
	read *snapshot
}

// GetHashType returns the preferred content hash without storage work.
func (s *packStore) GetHashType() hash.HashType {
	return s.hashType
}

// GetSupportedFeatures advertises native bounded block and existence batches.
func (s *packStore) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeatureNativeBatchPut | block.StoreFeatureNativeBatchExists
}

// BeginReadOperation protects one immutable generation across a bounded read.
func (s *packStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	if s.read != nil {
		return s, func() {}, nil
	}
	read, err := s.engine.snapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &packStore{engine: s.engine, hashType: s.hashType, read: read}, read.release, nil
}

// PutBlock validates a complete reference and durably inserts it if absent.
func (s *packStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if len(data) == 0 {
		return nil, false, block.ErrEmptyBlock
	}
	if len(data) > MaxValueBytes {
		return nil, false, ErrLimit
	}
	if opts == nil {
		opts = new(block.PutOpts)
	} else {
		opts = opts.CloneVT()
	}
	opts.HashType = opts.SelectHashType(s.hashType)
	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	if forced := opts.GetForceBlockRef(); !forced.GetEmpty() && !ref.EqualsRef(forced) {
		return ref, false, block.ErrBlockRefMismatch
	}
	existed, err := s.writePack(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data}})
	return ref, existed, err
}

// PutBlockBatch validates all inputs, then publishes bounded arrival-order packs.
func (s *packStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	for _, entry := range entries {
		if entry == nil {
			return block.ErrEmptyBlockRef
		}
		if err := entry.Ref.Validate(false); err != nil {
			return err
		}
		if !entry.Tombstone {
			if len(entry.Data) == 0 {
				return block.ErrEmptyBlock
			}
			if len(entry.Data) > MaxValueBytes {
				return ErrLimit
			}
			if err := entry.Ref.VerifyData(entry.Data, false); err != nil {
				return err
			}
		}
	}
	for len(entries) != 0 {
		count, size := 0, 0
		for count < len(entries) && count < maxPackRecords {
			entry := entries[count]
			key, err := blockKey(entry.Ref)
			if err != nil {
				return err
			}
			next := len(entry.Data) + len(key) + 12
			if count > 0 && (size+next > maxPackBytes || entry.Tombstone || entries[0].Tombstone) {
				break
			}
			size += next
			count++
		}
		if _, err := s.writePack(ctx, entries[:count]); err != nil {
			return err
		}
		entries = entries[count:]
	}
	return nil
}

// writePack resolves duplicate identities under the publication lock.
func (s *packStore) writePack(ctx context.Context, entries []*block.PutBatchEntry) (bool, error) {
	existed := false
	err := s.engine.mutate(ctx, func(read *snapshot, p *publication) ([]*Record, error) {
		p.root.Revision++
		changes := make(map[string]*Record)
		packs := make(map[string]*Pack)
		var payload []byte
		pack := new(Pack)
		for _, entry := range entries {
			key, err := blockKey(entry.Ref)
			if err != nil {
				return nil, err
			}
			current, found, err := read.get(ctx, key)
			if err != nil {
				return nil, err
			}
			if pending := changes[string(key)]; pending != nil {
				found = !pending.Deleted
				current = pending.Value
			}
			if entry.Tombstone {
				if found {
					if err := removeLocation(ctx, read, p, key, current, packs, changes); err != nil {
						return nil, err
					}
				}
				continue
			}
			if found {
				existed = true
				continue
			}

			// The pack carries full identity and framing beside each payload.
			checksum := crc32.ChecksumIEEE(entry.Data)
			payload = binary.LittleEndian.AppendUint32(payload, uint32(len(key)))        //nolint:gosec // pack payloads are bounded by maxPackBytes.
			payload = binary.LittleEndian.AppendUint32(payload, uint32(len(entry.Data))) //nolint:gosec // pack payloads are bounded by maxPackBytes.
			payload = binary.LittleEndian.AppendUint32(payload, checksum)
			payload = append(payload, key...)
			location := &Location{Offset: uint64(len(payload)), Length: uint32(len(entry.Data)), Checksum: checksum} //nolint:gosec // both lengths are bounded by maxPackBytes before encoding.
			payload = append(payload, entry.Data...)
			encoded, err := encode(location)
			if err != nil {
				return nil, err
			}
			record := &Record{Key: key, Value: encoded}
			pack.Records = append(pack.Records, record)
			pack.LiveBytes += uint64(len(entry.Data))
			changes[string(key)] = record
			p.root.BlockCount++
			p.root.BlockBytes += uint64(len(entry.Data))
		}
		if len(payload) != 0 {
			if len(payload) > maxPackBytes {
				return nil, ErrLimit
			}
			name, err := p.addBytes("pack", payload)
			if err != nil {
				return nil, err
			}
			for _, record := range pack.Records {
				location := new(Location)
				if err := decode(record.Value, location); err != nil {
					return nil, err
				}
				location.Pack = name
				location.PackBytes = uint32(len(payload)) //nolint:gosec // the preceding maxPackBytes check bounds the fixed-width location field.
				record.Value, err = encode(location)
				if err != nil {
					return nil, err
				}
			}
			packs[name] = pack
		}
		for name, pack := range packs {
			encoded, err := encode(pack)
			if err != nil {
				return nil, err
			}
			key := []byte(packPrefix + name)
			changes[string(key)] = &Record{Key: key, Value: encoded}
		}
		records := make([]*Record, 0, len(changes))
		for _, record := range changes {
			records = append(records, record)
		}
		return records, nil
	})
	return existed, err
}

// removeLocation updates the exact old pack and queues its first deletion.
func removeLocation(ctx context.Context, read *snapshot, p *publication, key, encoded []byte, packs map[string]*Pack, changes map[string]*Record) error {
	location := new(Location)
	if err := decode(encoded, location); err != nil {
		return err
	}
	pack := packs[location.Pack]
	if pack == nil {
		data, found, err := read.get(ctx, []byte(packPrefix+location.Pack))
		if err != nil {
			return err
		}
		if !found {
			return ErrCorrupt
		}
		pack = new(Pack)
		if err := decode(data, pack); err != nil {
			return err
		}
		packs[location.Pack] = pack
	}
	if pack.LiveBytes < uint64(location.Length) || p.root.BlockCount == 0 || p.root.BlockBytes < uint64(location.Length) {
		return ErrCorrupt
	}
	pack.LiveBytes -= uint64(location.Length)
	p.root.BlockCount--
	p.root.BlockBytes -= uint64(location.Length)
	if len(pack.CleanupKey) == 0 {
		pack.CleanupKey = binary.BigEndian.AppendUint64([]byte(cleanupPrefix), p.root.Generation)
		pack.CleanupKey = append(pack.CleanupKey, location.Pack...)
		changes[string(pack.CleanupKey)] = &Record{Key: pack.CleanupKey, Value: []byte(location.Pack)}
	}
	changes[string(key)] = &Record{Key: key, Deleted: true}
	return nil
}

// GetBlock resolves an index location before reading and checking payload bytes.
func (s *packStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	key, err := blockKey(ref)
	if err != nil {
		return nil, false, err
	}
	read, release, err := s.openRead(ctx)
	if err != nil {
		return nil, false, err
	}
	defer release()
	encoded, found, err := read.get(ctx, key)
	if err != nil || !found {
		return nil, found, err
	}
	location := new(Location)
	if err := decode(encoded, location); err != nil {
		return nil, false, err
	}
	data, err := s.engine.readLocation(ctx, location)
	return data, err == nil, err
}

// GetStoredBlock serves the block without refs because this store keeps
// block bytes without their refs.
func (s *packStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return block.GetBlockWithoutRefs(ctx, s, ref)
}

// openRead reuses an explicit bounded operation scope when supplied.
func (s *packStore) openRead(ctx context.Context) (*snapshot, func(), error) {
	if s.read != nil {
		return s.read, func() {}, nil
	}
	read, err := s.engine.snapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	return read, read.release, nil
}

// readLocation validates a bounded extent without opening any unrelated payload.
func (e *Engine) readLocation(ctx context.Context, location *Location) ([]byte, error) {
	if err := validateLocation(location); err != nil {
		return nil, err
	}
	const windowBytes = 64 << 10
	var data []byte
	if location.Length >= windowBytes {
		// Large payloads use one bounded range call and avoid displacing small-read windows.
		var err error
		data, err = e.backend.Read(ctx, location.Pack, int64(location.Offset), int(location.Length)) //nolint:gosec // validateLocation bounds offset and length by the 4 MiB pack limit.
		if err != nil {
			return nil, err
		}
	} else {
		// Small reads reuse adjacent extents while charging windows to the shared budget.
		data = make([]byte, int(location.Length))
		end := location.Offset + uint64(location.Length)
		for offset := location.Offset; offset < end; {
			base := offset / windowBytes * windowBytes
			size := min(uint64(windowBytes), uint64(location.PackBytes)-base)
			key := location.Pack + ":" + strconv.FormatUint(base, 10)
			window, err := e.readCached(ctx, key, location.Pack, int64(base), int(size)) //nolint:gosec // validated pack metadata bounds base and size by maxPackBytes.
			if err != nil {
				return nil, err
			}
			if len(window) != int(size) {
				return nil, ErrCorrupt
			}
			count := min(end-offset, base+size-offset)
			copy(data[offset-location.Offset:], window[offset-base:offset-base+count])
			offset += count
		}
	}
	if len(data) != int(location.Length) || crc32.ChecksumIEEE(data) != location.Checksum {
		return nil, ErrCorrupt
	}
	return data, nil
}

// validateLocation rejects extents outside the bounded immutable pack.
func validateLocation(location *Location) error {
	if location.Pack == "" || location.Length == 0 || location.Length > MaxValueBytes || location.PackBytes == 0 || location.PackBytes > maxPackBytes || location.Offset > uint64(location.PackBytes) || uint64(location.Length) > uint64(location.PackBytes)-location.Offset {
		return ErrCorrupt
	}
	return nil
}

// GetBlockExists answers entirely from small index records.
func (s *packStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	stat, err := s.StatBlock(ctx, ref)
	return stat != nil, err
}

// GetBlockExistsBatch shares root validation and file protection across the batch.
func (s *packStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	read, release, err := s.openRead(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	out := make([]bool, len(refs))
	for i, ref := range refs {
		if ref.GetEmpty() {
			continue
		}
		key, err := blockKey(ref)
		if err != nil {
			return nil, err
		}
		_, out[i], err = read.get(ctx, key)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RmBlock atomically hides a location and accounts its exact old extent as dead.
func (s *packStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if err := ref.Validate(false); err != nil {
		return err
	}
	_, err := s.writePack(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}})
	return err
}

// StatBlock reads the payload length from the location index alone.
func (s *packStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	key, err := blockKey(ref)
	if err != nil {
		return nil, err
	}
	read, release, err := s.openRead(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	encoded, found, err := read.get(ctx, key)
	if err != nil || !found {
		return nil, err
	}
	location := new(Location)
	if err := decode(encoded, location); err != nil {
		return nil, err
	}
	return &block.BlockStat{Ref: ref.Clone(), Size: int64(location.Length)}, nil
}

// Sync reports the durable contract of completed pack publications.
func (s *packStore) Sync(ctx context.Context) (bool, error) {
	_, err := s.engine.RefreshGenerationContext(ctx)
	return err == nil, err
}

// blockKey retains the complete binary BlockRef identity in the ordered index.
func blockKey(ref *block.BlockRef) ([]byte, error) {
	if err := ref.Validate(false); err != nil {
		return nil, err
	}
	key, err := ref.MarshalKey()
	if err != nil {
		return nil, err
	}
	return append([]byte(blockPrefix), key...), nil
}

// sameLocation compares complete location versions during conditional relocation.
func sameLocation(current, expected []byte) bool {
	return bytes.Equal(current, expected)
}

// _ verifies the existing block store contract.
var _ block.StoreOps = (*packStore)(nil)

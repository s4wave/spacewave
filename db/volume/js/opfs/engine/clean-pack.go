package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
)

// preparedExtent holds one live source record copied outside the publication lock.
type preparedExtent struct {
	// record carries the complete source identity and exact old location version.
	record *Record
	// data contains the checksummed payload copied under reader protection.
	data []byte
}

// CleanPack relocates one oldest dirty pack and atomically advances its queue.
// Preparation does not hold the publication lock; each copied location is
// compared again before it can replace a current index entry.
func (e *Engine) CleanPack(ctx context.Context) (bool, error) {
	key, name, extents, err := e.preparePack(ctx)
	if err != nil || key == nil {
		return false, err
	}
	progress := false
	err = e.mutate(ctx, func(read *snapshot, p *publication) ([]*Record, error) {
		queued, found, err := read.get(ctx, key)
		if err != nil || !found {
			return nil, err
		}
		if string(queued) != name {
			return nil, ErrCorrupt
		}
		data, found, err := read.get(ctx, []byte(packPrefix+name))
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, ErrCorrupt
		}
		source := new(Pack)
		if err := decode(data, source); err != nil {
			return nil, err
		}

		// Keep only extents that still point to exactly the copied source version.
		var payload []byte
		pack := new(Pack)
		for _, extent := range extents {
			current, found, err := read.get(ctx, extent.record.Key)
			if err != nil {
				return nil, err
			}
			if !found || !sameLocation(current, extent.record.Value) {
				continue
			}
			location := new(Location)
			if err := decode(current, location); err != nil {
				return nil, err
			}
			payload = binary.LittleEndian.AppendUint32(payload, uint32(len(extent.record.Key))) //nolint:gosec // relocation payloads are bounded by maxPackBytes.
			payload = binary.LittleEndian.AppendUint32(payload, location.Length)
			payload = binary.LittleEndian.AppendUint32(payload, location.Checksum)
			payload = append(payload, extent.record.Key...)
			location.Offset = uint64(len(payload))
			payload = append(payload, extent.data...)
			encoded, err := encode(location)
			if err != nil {
				return nil, err
			}
			pack.Records = append(pack.Records, &Record{Key: extent.record.Key, Value: encoded})
			pack.LiveBytes += uint64(location.Length)
		}
		if pack.LiveBytes != source.LiveBytes || len(payload) > maxPackBytes {
			return nil, ErrCorrupt
		}
		records := []*Record{{Key: key, Deleted: true}, {Key: []byte(packPrefix + name), Deleted: true}}
		if len(payload) != 0 {
			newName, err := p.addBytes("pack", payload)
			if err != nil {
				return nil, err
			}
			for _, record := range pack.Records {
				location := new(Location)
				if err := decode(record.Value, location); err != nil {
					return nil, err
				}
				location.Pack = newName
				location.PackBytes = uint32(len(payload)) //nolint:gosec // the preceding maxPackBytes check bounds the fixed-width location field.
				record.Value, err = encode(location)
				if err != nil {
					return nil, err
				}
				records = append(records, record)
			}
			encoded, err := encode(pack)
			if err != nil {
				return nil, err
			}
			records = append(records, &Record{Key: []byte(packPrefix + newName), Value: encoded})
		}
		p.retire(name)
		progress = true
		return records, nil
	})
	return progress && err == nil, err
}

// preparePack copies one bounded pack's still-live extents with file protection.
func (e *Engine) preparePack(ctx context.Context) ([]byte, string, []preparedExtent, error) {
	read, err := e.snapshot(ctx)
	if err != nil {
		return nil, "", nil, err
	}
	defer read.release()
	records, err := read.seekEntries(ctx, []byte(cleanupPrefix), false, false)
	if err != nil {
		return nil, "", nil, err
	}
	if len(records) == 0 || !bytes.HasPrefix(records[0].Key, []byte(cleanupPrefix)) {
		return nil, "", nil, nil
	}
	key, name := records[0].Key, string(records[0].Value)
	data, found, err := read.get(ctx, []byte(packPrefix+name))
	if err != nil {
		return nil, "", nil, err
	}
	if !found {
		return nil, "", nil, ErrCorrupt
	}
	pack := new(Pack)
	if err := decode(data, pack); err != nil {
		return nil, "", nil, err
	}
	if len(pack.Records) > maxPackRecords || pack.LiveBytes > maxPackBytes || !bytes.Equal(pack.CleanupKey, key) {
		return nil, "", nil, ErrCorrupt
	}
	// Open a source pack only once; copied extents retain this bounded byte slice.
	var payload []byte
	if pack.LiveBytes != 0 {
		payload, err = e.backend.Read(ctx, name, 0, readAll)
		if err != nil {
			return nil, "", nil, err
		}
		if len(payload) > maxPackBytes {
			return nil, "", nil, ErrCorrupt
		}
	}
	var extents []preparedExtent
	var liveBytes uint64
	for _, record := range pack.Records {
		if record == nil {
			return nil, "", nil, ErrCorrupt
		}
		current, found, err := read.get(ctx, record.Key)
		if err != nil {
			return nil, "", nil, err
		}
		if !found || !sameLocation(current, record.Value) {
			continue
		}
		location := new(Location)
		if err := decode(current, location); err != nil {
			return nil, "", nil, err
		}
		if location.Pack != name {
			return nil, "", nil, ErrCorrupt
		}
		if err := validateLocation(location); err != nil {
			return nil, "", nil, err
		}
		if int(location.PackBytes) != len(payload) {
			return nil, "", nil, ErrCorrupt
		}
		data := payload[location.Offset : location.Offset+uint64(location.Length)]
		if crc32.ChecksumIEEE(data) != location.Checksum {
			return nil, "", nil, ErrCorrupt
		}
		liveBytes += uint64(len(data))
		if liveBytes > maxPackBytes {
			return nil, "", nil, ErrCorrupt
		}
		extents = append(extents, preparedExtent{record: record, data: data})
	}
	if liveBytes != pack.LiveBytes {
		return nil, "", nil, ErrCorrupt
	}
	return key, name, extents, nil
}

// BlockStats returns committed block count and live payload bytes.
func (e *Engine) BlockStats(ctx context.Context) (uint64, uint64, error) {
	read, err := e.snapshot(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer read.release()
	return read.root.BlockCount, read.root.BlockBytes, nil
}

// Maintenance performs one bounded relocation and retirement quantum.
func (e *Engine) Maintenance(ctx context.Context) error {
	if _, err := e.CleanPack(ctx); err != nil {
		return err
	}
	_, err := e.Reclaim(ctx)
	return err
}

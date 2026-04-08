// Package blockshard implements a sharded block store engine backed by
// immutable SSTable segment files with per-shard manifests.
package blockshard

import (
	"encoding/binary"
	"hash/crc32"

	"github.com/pkg/errors"
)

// ManifestMagic identifies a manifest file.
var ManifestMagic = [4]byte{'B', 'S', 'M', 'F'}

// ManifestHeaderSize is the fixed manifest header size.
const ManifestHeaderSize = 16

// Manifest is the per-shard manifest listing active segment files.
// Double-buffered: two manifest slots per shard, higher valid generation wins.
type Manifest struct {
	Generation uint64
	Segments   []SegmentMeta
}

// SegmentMeta describes one SSTable segment file in the shard.
type SegmentMeta struct {
	// Filename is the segment file name within the shard directory.
	Filename string
	// EntryCount is the number of entries in the segment.
	EntryCount uint32
	// Size is the file size in bytes.
	Size uint32
	// Level is the compaction level (0 = freshly flushed).
	Level uint8
	// MinKey is the smallest key in the segment.
	MinKey []byte
	// MaxKey is the largest key in the segment.
	MaxKey []byte
}

// Encode serializes the manifest to bytes with a CRC32 footer.
//
// Layout:
//
//	[magic: 4] [version: u16] [reserved: u16] [generation: u64]
//	[segment_count: u32]
//	per segment:
//	  [filename_len: u16] [filename]
//	  [entry_count: u32] [size: u32] [level: u8]
//	  [min_key_len: u16] [min_key]
//	  [max_key_len: u16] [max_key]
//	[crc32: u32]
func (m *Manifest) Encode() []byte {
	// Compute body size.
	size := ManifestHeaderSize + 4 // header + segment_count
	for i := range m.Segments {
		s := &m.Segments[i]
		size += 2 + len(s.Filename) + 4 + 4 + 1 + 2 + len(s.MinKey) + 2 + len(s.MaxKey)
	}
	size += 4 // CRC32 footer

	buf := make([]byte, size)
	off := 0

	// Header.
	copy(buf[off:off+4], ManifestMagic[:])
	off += 4
	binary.BigEndian.PutUint16(buf[off:off+2], 1) // version
	off += 2
	off += 2 // reserved
	binary.BigEndian.PutUint64(buf[off:off+8], m.Generation)
	off += 8

	// Segment count.
	binary.BigEndian.PutUint32(buf[off:off+4], uint32(len(m.Segments)))
	off += 4

	// Segments.
	for i := range m.Segments {
		s := &m.Segments[i]
		binary.BigEndian.PutUint16(buf[off:off+2], uint16(len(s.Filename)))
		off += 2
		copy(buf[off:], s.Filename)
		off += len(s.Filename)
		binary.BigEndian.PutUint32(buf[off:off+4], s.EntryCount)
		off += 4
		binary.BigEndian.PutUint32(buf[off:off+4], s.Size)
		off += 4
		buf[off] = s.Level
		off++
		binary.BigEndian.PutUint16(buf[off:off+2], uint16(len(s.MinKey)))
		off += 2
		copy(buf[off:], s.MinKey)
		off += len(s.MinKey)
		binary.BigEndian.PutUint16(buf[off:off+2], uint16(len(s.MaxKey)))
		off += 2
		copy(buf[off:], s.MaxKey)
		off += len(s.MaxKey)
	}

	// CRC32 footer (checksum of everything before the footer).
	crc := crc32.ChecksumIEEE(buf[:off])
	binary.BigEndian.PutUint32(buf[off:off+4], crc)

	return buf
}

// DecodeManifest parses a manifest from bytes, validating the CRC32 footer.
func DecodeManifest(buf []byte) (*Manifest, error) {
	if len(buf) < ManifestHeaderSize+4+4 {
		return nil, errors.New("manifest too short")
	}

	// Verify CRC32.
	contentLen := len(buf) - 4
	expected := binary.BigEndian.Uint32(buf[contentLen:])
	actual := crc32.ChecksumIEEE(buf[:contentLen])
	if expected != actual {
		return nil, errors.Errorf("manifest CRC32 mismatch: expected %08x, got %08x", expected, actual)
	}

	off := 0

	// Header.
	var magic [4]byte
	copy(magic[:], buf[off:off+4])
	if magic != ManifestMagic {
		return nil, errors.Errorf("invalid manifest magic: %x", magic)
	}
	off += 4
	version := binary.BigEndian.Uint16(buf[off : off+2])
	if version != 1 {
		return nil, errors.Errorf("unsupported manifest version: %d", version)
	}
	off += 2
	off += 2 // reserved
	gen := binary.BigEndian.Uint64(buf[off : off+8])
	off += 8

	// Segment count.
	if off+4 > contentLen {
		return nil, errors.New("truncated segment count")
	}
	count := binary.BigEndian.Uint32(buf[off : off+4])
	off += 4

	segments := make([]SegmentMeta, count)
	for i := range segments {
		if off+2 > contentLen {
			return nil, errors.Errorf("truncated segment %d filename length", i)
		}
		fnLen := int(binary.BigEndian.Uint16(buf[off : off+2]))
		off += 2
		if off+fnLen > contentLen {
			return nil, errors.Errorf("truncated segment %d filename", i)
		}
		segments[i].Filename = string(buf[off : off+fnLen])
		off += fnLen

		if off+9 > contentLen {
			return nil, errors.Errorf("truncated segment %d metadata", i)
		}
		segments[i].EntryCount = binary.BigEndian.Uint32(buf[off : off+4])
		off += 4
		segments[i].Size = binary.BigEndian.Uint32(buf[off : off+4])
		off += 4
		segments[i].Level = buf[off]
		off++

		if off+2 > contentLen {
			return nil, errors.Errorf("truncated segment %d min key length", i)
		}
		mkLen := int(binary.BigEndian.Uint16(buf[off : off+2]))
		off += 2
		if off+mkLen > contentLen {
			return nil, errors.Errorf("truncated segment %d min key", i)
		}
		segments[i].MinKey = make([]byte, mkLen)
		copy(segments[i].MinKey, buf[off:off+mkLen])
		off += mkLen

		if off+2 > contentLen {
			return nil, errors.Errorf("truncated segment %d max key length", i)
		}
		mxLen := int(binary.BigEndian.Uint16(buf[off : off+2]))
		off += 2
		if off+mxLen > contentLen {
			return nil, errors.Errorf("truncated segment %d max key", i)
		}
		segments[i].MaxKey = make([]byte, mxLen)
		copy(segments[i].MaxKey, buf[off:off+mxLen])
		off += mxLen
	}

	return &Manifest{
		Generation: gen,
		Segments:   segments,
	}, nil
}

// PickManifest selects the valid manifest with the higher generation.
// Returns nil if both are invalid.
func PickManifest(a, b []byte) *Manifest {
	ma, errA := DecodeManifest(a)
	mb, errB := DecodeManifest(b)
	if errA != nil && errB != nil {
		return nil
	}
	if errA != nil {
		return mb
	}
	if errB != nil {
		return ma
	}
	if mb.Generation > ma.Generation {
		return mb
	}
	return ma
}

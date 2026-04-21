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

const (
	manifestVersion1 = 1
	manifestVersion2 = 2
)

// Manifest is the per-shard manifest listing active segment files.
// Double-buffered: two manifest slots per shard, higher valid generation wins.
type Manifest struct {
	Generation    uint64
	Segments      []SegmentMeta
	PendingDelete []RetiredSegmentMeta
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

// RetiredSegmentMeta describes a segment retired from the active manifest but
// still retained on disk for stale-reader safety and crash recovery.
type RetiredSegmentMeta struct {
	SegmentMeta
	// RetireGeneration is the manifest generation where the segment left the
	// active set.
	RetireGeneration uint64
	// DeleteAfterUnixMilli is the earliest wall-clock time when the segment may
	// be reclaimed, subject to additional generation checks.
	DeleteAfterUnixMilli uint64
}

// Encode serializes the manifest to bytes with a CRC32 footer.
//
// Layout:
//
//	[magic: 4] [version: u16] [reserved: u16] [generation: u64]
//	[segment_count: u32] [pending_delete_count: u32]
//	per active segment:
//	  [filename_len: u16] [filename]
//	  [entry_count: u32] [size: u32] [level: u8]
//	  [min_key_len: u16] [min_key]
//	  [max_key_len: u16] [max_key]
//	per pending-delete segment:
//	  [filename_len: u16] [filename]
//	  [entry_count: u32] [size: u32] [level: u8]
//	  [min_key_len: u16] [min_key]
//	  [max_key_len: u16] [max_key]
//	  [retire_generation: u64] [delete_after_unix_milli: u64]
//	[crc32: u32]
func (m *Manifest) Encode() []byte {
	size := ManifestHeaderSize + 8 // header + active/pending counts
	for i := range m.Segments {
		size += encodedSegmentMetaSize(&m.Segments[i])
	}
	for i := range m.PendingDelete {
		size += encodedRetiredSegmentMetaSize(&m.PendingDelete[i])
	}
	size += 4 // CRC32 footer

	buf := make([]byte, size)
	off := 0

	// Header.
	copy(buf[off:off+4], ManifestMagic[:])
	off += 4
	binary.BigEndian.PutUint16(buf[off:off+2], manifestVersion2)
	off += 2
	off += 2 // reserved
	binary.BigEndian.PutUint64(buf[off:off+8], m.Generation)
	off += 8

	// Segment counts.
	binary.BigEndian.PutUint32(buf[off:off+4], uint32(len(m.Segments)))
	off += 4
	binary.BigEndian.PutUint32(buf[off:off+4], uint32(len(m.PendingDelete)))
	off += 4

	for i := range m.Segments {
		off = encodeSegmentMeta(buf, off, &m.Segments[i])
	}
	for i := range m.PendingDelete {
		off = encodeRetiredSegmentMeta(buf, off, &m.PendingDelete[i])
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
	if version != manifestVersion1 && version != manifestVersion2 {
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
	var pendingCount uint32
	if version >= manifestVersion2 {
		if off+4 > contentLen {
			return nil, errors.New("truncated pending-delete count")
		}
		pendingCount = binary.BigEndian.Uint32(buf[off : off+4])
		off += 4
	}

	segments := make([]SegmentMeta, count)
	for i := range segments {
		next, err := decodeSegmentMeta(buf, off, contentLen, &segments[i], i)
		if err != nil {
			return nil, err
		}
		off = next
	}
	pending := make([]RetiredSegmentMeta, pendingCount)
	for i := range pending {
		next, err := decodeRetiredSegmentMeta(buf, off, contentLen, &pending[i], i)
		if err != nil {
			return nil, err
		}
		off = next
	}

	return &Manifest{
		Generation:    gen,
		Segments:      segments,
		PendingDelete: pending,
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

// Clone returns a deep copy of the manifest.
func (m *Manifest) Clone() *Manifest {
	if m == nil {
		return nil
	}
	out := &Manifest{
		Generation:    m.Generation,
		Segments:      make([]SegmentMeta, len(m.Segments)),
		PendingDelete: make([]RetiredSegmentMeta, len(m.PendingDelete)),
	}
	for i := range m.Segments {
		out.Segments[i] = cloneSegmentMeta(m.Segments[i])
	}
	for i := range m.PendingDelete {
		out.PendingDelete[i] = cloneRetiredSegmentMeta(m.PendingDelete[i])
	}
	return out
}

// ReferencedFiles returns all segment filenames referenced by the manifest.
func (m *Manifest) ReferencedFiles() map[string]struct{} {
	refs := make(map[string]struct{}, len(m.Segments)+len(m.PendingDelete))
	for i := range m.Segments {
		refs[m.Segments[i].Filename] = struct{}{}
	}
	for i := range m.PendingDelete {
		refs[m.PendingDelete[i].Filename] = struct{}{}
	}
	return refs
}

func encodedSegmentMetaSize(s *SegmentMeta) int {
	return 2 + len(s.Filename) + 4 + 4 + 1 + 2 + len(s.MinKey) + 2 + len(s.MaxKey)
}

func encodedRetiredSegmentMetaSize(s *RetiredSegmentMeta) int {
	return encodedSegmentMetaSize(&s.SegmentMeta) + 8 + 8
}

func encodeSegmentMeta(buf []byte, off int, s *SegmentMeta) int {
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
	return off
}

func encodeRetiredSegmentMeta(buf []byte, off int, s *RetiredSegmentMeta) int {
	off = encodeSegmentMeta(buf, off, &s.SegmentMeta)
	binary.BigEndian.PutUint64(buf[off:off+8], s.RetireGeneration)
	off += 8
	binary.BigEndian.PutUint64(buf[off:off+8], s.DeleteAfterUnixMilli)
	off += 8
	return off
}

func decodeSegmentMeta(
	buf []byte,
	off, contentLen int,
	seg *SegmentMeta,
	idx int,
) (int, error) {
	if off+2 > contentLen {
		return off, errors.Errorf("truncated segment %d filename length", idx)
	}
	fnLen := int(binary.BigEndian.Uint16(buf[off : off+2]))
	off += 2
	if off+fnLen > contentLen {
		return off, errors.Errorf("truncated segment %d filename", idx)
	}
	seg.Filename = string(buf[off : off+fnLen])
	off += fnLen

	if off+9 > contentLen {
		return off, errors.Errorf("truncated segment %d metadata", idx)
	}
	seg.EntryCount = binary.BigEndian.Uint32(buf[off : off+4])
	off += 4
	seg.Size = binary.BigEndian.Uint32(buf[off : off+4])
	off += 4
	seg.Level = buf[off]
	off++

	if off+2 > contentLen {
		return off, errors.Errorf("truncated segment %d min key length", idx)
	}
	mkLen := int(binary.BigEndian.Uint16(buf[off : off+2]))
	off += 2
	if off+mkLen > contentLen {
		return off, errors.Errorf("truncated segment %d min key", idx)
	}
	seg.MinKey = make([]byte, mkLen)
	copy(seg.MinKey, buf[off:off+mkLen])
	off += mkLen

	if off+2 > contentLen {
		return off, errors.Errorf("truncated segment %d max key length", idx)
	}
	mxLen := int(binary.BigEndian.Uint16(buf[off : off+2]))
	off += 2
	if off+mxLen > contentLen {
		return off, errors.Errorf("truncated segment %d max key", idx)
	}
	seg.MaxKey = make([]byte, mxLen)
	copy(seg.MaxKey, buf[off:off+mxLen])
	off += mxLen
	return off, nil
}

func decodeRetiredSegmentMeta(
	buf []byte,
	off, contentLen int,
	seg *RetiredSegmentMeta,
	idx int,
) (int, error) {
	next, err := decodeSegmentMeta(buf, off, contentLen, &seg.SegmentMeta, idx)
	if err != nil {
		return off, err
	}
	if next+16 > contentLen {
		return off, errors.Errorf("truncated retired segment %d metadata", idx)
	}
	seg.RetireGeneration = binary.BigEndian.Uint64(buf[next : next+8])
	next += 8
	seg.DeleteAfterUnixMilli = binary.BigEndian.Uint64(buf[next : next+8])
	next += 8
	return next, nil
}

func cloneSegmentMeta(s SegmentMeta) SegmentMeta {
	s.MinKey = append([]byte{}, s.MinKey...)
	s.MaxKey = append([]byte{}, s.MaxKey...)
	return s
}

func cloneRetiredSegmentMeta(s RetiredSegmentMeta) RetiredSegmentMeta {
	s.SegmentMeta = cloneSegmentMeta(s.SegmentMeta)
	return s
}

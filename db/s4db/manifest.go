package s4db

import (
	"encoding/binary"
	"slices"

	"github.com/pkg/errors"
)

// manifest is one version of the file's structure: its runs and its chunk
// table, with the key count and the open value chunks. The writer derives
// the next version from the previous one and a record; every process derives
// the same versions from the same records.
type manifest struct {
	// version increases with each record that changes the structure.
	version uint64
	// runs holds every run, level 0 newest first, then each lower level in
	// key order.
	runs []runMeta
	// table is the chunk table.
	table *chunkTable
	// count is the number of keys.
	count int64
	// hot and cold are the open value chunks, zero before the first value.
	hot, cold uint64
}

// newManifest returns the manifest of an empty file: the header chunk and
// nothing else.
func newManifest() *manifest {
	return &manifest{version: 1, table: newChunkTable()}
}

// clone returns a copy the caller may change.
func (m *manifest) clone() *manifest {
	return &manifest{
		version: m.version,
		runs:    slices.Clone(m.runs),
		table:   m.table.clone(),
		count:   m.count,
		hot:     m.hot,
		cold:    m.cold,
	}
}

// encode returns the manifest as one checksummed record.
func (m *manifest) encode() []byte {
	// Write the fields.
	b := make([]byte, 0, 64+len(m.runs)*64+len(m.table.entries)*chunkEntrySize)
	b = binary.AppendUvarint(b, m.version)
	b = binary.AppendVarint(b, m.count)
	b = binary.AppendUvarint(b, m.hot)
	b = binary.AppendUvarint(b, m.cold)
	b = appendRunList(b, m.runs)
	b = m.table.encode(b)

	// Seal the record.
	return binary.LittleEndian.AppendUint32(b, checksum(b))
}

// decodeManifest reads a manifest record.
func decodeManifest(b []byte) (*manifest, error) {
	// Check the checksum.
	if len(b) < 4 || checksum(b[:len(b)-4]) != binary.LittleEndian.Uint32(b[len(b)-4:]) {
		return nil, errors.Wrap(ErrCorrupt, "manifest checksum mismatch")
	}

	// Decode the fields.
	d := decoder{b: b[:len(b)-4]}
	m := &manifest{version: d.uvarint(), count: d.varint(), hot: d.uvarint(), cold: d.uvarint()}
	m.runs = d.runList()
	m.table = decodeChunkTable(&d)

	// Refuse a truncated record.
	if d.err != nil {
		return nil, errors.Wrap(ErrCorrupt, "manifest truncated")
	}
	return m, nil
}

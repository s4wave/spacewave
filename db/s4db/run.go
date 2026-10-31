package s4db

import (
	"encoding/binary"
)

// runMeta describes one sorted run as the manifest and the flush and merge
// records carry it. A run is a sorted sequence of whole chunks holding data
// blocks, index partitions, filter partitions and a footer in its last chunk.
type runMeta struct {
	// id identifies the run: the sequence of the record that added it, plus
	// its index among that record's runs.
	id uint64
	// level is the run's level; level 0 runs may overlap.
	level uint8
	// chunks are the run's chunks in order.
	chunks []uint64
	// count is the number of entries, tombstones included.
	count uint64
	// smallest and largest bound the run's keys.
	smallest, largest []byte
	// footer locates the run's footer.
	footer ref
}

// decode reads a run description.
func (m *runMeta) decode(d *decoder) {
	// Read the identity, chunks, count, bounds and footer.
	m.id = d.uvarint()
	m.level = d.kind()
	m.chunks = d.chunkList()
	m.count = d.uvarint()
	m.smallest = d.bytes()
	m.largest = d.bytes()
	m.footer = ref{off: d.uvarint(), n: uint32(d.uvarint()), crc: d.u32()} // #nosec G115 -- append wrote a uint32.
}

// append encodes m onto b.
func (m *runMeta) append(b []byte) []byte {
	// Write the identity, chunks and count.
	b = binary.AppendUvarint(b, m.id)
	b = append(b, m.level)
	b = appendChunkList(b, m.chunks)
	b = binary.AppendUvarint(b, m.count)

	// Write the bounds and the footer location.
	b = binary.AppendUvarint(b, uint64(len(m.smallest)))
	b = append(b, m.smallest...)
	b = binary.AppendUvarint(b, uint64(len(m.largest)))
	b = append(b, m.largest...)
	b = binary.AppendUvarint(b, m.footer.off)
	b = binary.AppendUvarint(b, uint64(m.footer.n))
	return binary.LittleEndian.AppendUint32(b, m.footer.crc)
}

// bound returns an upper bound on m's encoded length.
func (m *runMeta) bound() int {
	return 8*binary.MaxVarintLen64 + 1 + binary.MaxVarintLen64*len(m.chunks) + len(m.smallest) + len(m.largest)
}

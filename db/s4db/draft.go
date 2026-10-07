package s4db

import (
	"encoding/binary"
	"slices"
)

// draft is a tree page a checkpoint assembles before it has a page number.
type draft struct {
	// leaf is set for leaf pages.
	leaf bool
	// keys holds the leaf keys, or the low key of each child of an inner
	// page. The first inner key is empty.
	keys [][]byte
	// vals holds the leaf values.
	vals []value
	// kids holds the child pages of an inner page, zero where the child is
	// a new page in sub until place assigns it.
	kids []uint64
	// sub holds the new children of an inner page, nil where the child is
	// an existing page.
	sub []*draft
}

// draftOf copies the entries of a decoded page into a draft.
func draftOf(n *node) *draft {
	// Size the draft for its kind and count.
	count := n.count()
	d := &draft{leaf: n.leaf, keys: make([][]byte, count)}
	if n.leaf {
		d.vals = make([]value, count)
	} else {
		d.kids = make([]uint64, count)
		d.sub = make([]*draft, count)
	}

	// Copy each key with its value or child page.
	for i := range count {
		d.keys[i] = n.key(i)
		if n.leaf {
			d.vals[i] = n.val(i)
			continue
		}
		d.kids[i] = n.kid(i)
	}
	return d
}

// fill returns the bytes the entries of d occupy.
func (d *draft) fill() int {
	total := 0
	for i, k := range d.keys {
		if d.leaf {
			total += leafEntrySize(k, d.vals[i])
			continue
		}
		total += innerEntrySize(k)
	}
	return total
}

// encode appends the page holding d to dst.
func (d *draft) encode(dst []byte) []byte {
	// Write the kind and count.
	start := len(dst)
	b := slices.Grow(dst, pageSize)[:start+pageHeader]
	b[start] = pageInner
	if d.leaf {
		b[start] = pageLeaf
	}
	binary.LittleEndian.PutUint16(b[start+1:], uint16(len(d.keys))) // #nosec G115 -- a page holds under 1<<16 entries.

	// Write each key with its value or child page.
	for i, k := range d.keys {
		b = binary.AppendUvarint(b, uint64(len(k)))
		b = append(b, k...)
		if d.leaf {
			b = d.vals[i].append(b)
			continue
		}
		b = binary.LittleEndian.AppendUint64(b, d.kids[i])
	}

	// Pad the page and seal it.
	end := len(b)
	b = b[:start+pageSize]
	clear(b[end:])
	page := b[start:]
	binary.LittleEndian.PutUint32(page[pageSize-4:], checksum(page[:pageSize-4]))
	return b
}

package s4dbv1

import (
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

// node is a decoded tree page: the page bytes and the offset of each entry.
type node struct {
	// leaf is set for leaf pages.
	leaf bool
	// page is the encoded page.
	page []byte
	// offs holds the offset of each entry in page.
	offs []uint16
}

// readNode reads and decodes the tree page at page.
func readNode(r io.ReaderAt, page uint64) (*node, error) {
	// Read the page and decode it.
	b := make([]byte, pageSize)
	if _, err := r.ReadAt(b, pageOff(page)); err != nil {
		return nil, err
	}
	n, err := decodeNode(b)
	if err != nil {
		return nil, errors.Wrapf(err, "page %d", page)
	}
	return n, nil
}

// decodeNode checks a page and finds its entries. A leaf entry is a key, a
// kind byte and a value; an inner entry is a key and a child page, and the
// first inner key is empty.
func decodeNode(b []byte) (*node, error) {
	// Check the checksum and the kind.
	if checksum(b[:pageSize-4]) != binary.LittleEndian.Uint32(b[pageSize-4:]) {
		return nil, errors.Wrap(ErrCorrupt, "page checksum mismatch")
	}
	if b[0] != pageInner && b[0] != pageLeaf {
		return nil, errors.Wrap(ErrCorrupt, "unknown page kind")
	}

	// Find each entry, checking that it is whole.
	count := int(binary.LittleEndian.Uint16(b[1:]))
	n := &node{leaf: b[0] == pageLeaf, page: b, offs: make([]uint16, count)}
	d := decoder{b: b[pageHeader : pageSize-4]}
	for i := range count {
		n.offs[i] = uint16(pageSize - 4 - len(d.b)) // #nosec G115 -- offsets fall within a page.
		d.bytes()
		if n.leaf {
			d.value(d.kind())
			continue
		}
		d.u64()
	}
	if d.err != nil {
		return nil, d.err
	}
	return n, nil
}

// entry returns a decoder at entry i.
func (n *node) entry(i int) decoder {
	return decoder{b: n.page[n.offs[i] : pageSize-4]}
}

// pair returns the key and value of leaf entry i.
func (n *node) pair(i int) ([]byte, value) {
	d := n.entry(i)
	k := d.bytes()
	return k, d.value(d.kind())
}

// kid returns the child page of inner entry i.
func (n *node) kid(i int) uint64 {
	d := n.entry(i)
	d.bytes()
	return d.u64()
}

// treeWalk streams a tree's leaf entries in key order, holding one page per
// level.
type treeWalk struct {
	// r reads the file.
	r io.ReaderAt
	// stack holds the path from the root, each with the next entry to visit.
	stack []frame
	// err is the first read error.
	err error
}

// frame is one page on the walk's path.
type frame struct {
	// n is the page.
	n *node
	// i is the next entry to visit.
	i int
}

// newTreeWalk returns a walk of the tree at root; root zero is empty.
func newTreeWalk(r io.ReaderAt, root uint64) *treeWalk {
	w := &treeWalk{r: r}
	if root != 0 {
		w.push(root)
	}
	return w
}

// push reads page onto the path.
func (w *treeWalk) push(page uint64) {
	n, err := readNode(w.r, page)
	if err != nil {
		w.err = err
		w.stack = nil
		return
	}
	w.stack = append(w.stack, frame{n: n})
}

// next returns the next leaf entry, or false at the end or on error. The
// key and an inline value point into the page and last until the next call.
func (w *treeWalk) next() ([]byte, value, bool) {
	for len(w.stack) != 0 {
		// Leave a finished page, take a leaf entry, or descend.
		top := &w.stack[len(w.stack)-1]
		switch {
		case top.i >= len(top.n.offs):
			w.stack = w.stack[:len(w.stack)-1]
		case top.n.leaf:
			k, v := top.n.pair(top.i)
			top.i++
			return k, v, true
		default:
			kid := top.n.kid(top.i)
			top.i++
			w.push(kid)
		}
	}
	return nil, value{}, false
}

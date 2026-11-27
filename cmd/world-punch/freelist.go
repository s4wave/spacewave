package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math"
	"math/bits"

	"github.com/pkg/errors"
)

// pendingByte is SQLite's PENDING_BYTE. The page holding it is the lock-byte
// page, which SQLite never uses and never puts on the freelist.
const pendingByte = 0x40000000

// header is the part of the SQLite database header the punch needs.
type header struct {
	pageSize  uint32
	usable    uint32
	dbPages   uint32
	trunk     uint32
	freeCount uint32
	autoVac   bool
}

// pageBytes returns the byte length of n pages. A uint32 page count times
// a page size of at most 65536 fits in int64.
func (h header) pageBytes(n uint32) int64 { return int64(n) * int64(h.pageSize) }

// lockPage returns the number of the page containing SQLite's lock bytes.
func (h header) lockPage() uint32 { return pendingByte/h.pageSize + 1 }

// readHeader reads and checks the 100-byte header of page 1. It returns a
// zero header when the page layout is invalid. A stale count or a file size
// mismatch returns the decoded layout with an error for live readers.
func readHeader(r io.ReaderAt, fileSize int64) (header, error) {
	// Read the SQLite header before decoding any of its fields.
	var b [100]byte
	if _, err := r.ReadAt(b[:], 0); err != nil {
		return header{}, errors.Wrap(err, "read header")
	}
	if string(b[:16]) != "SQLite format 3\x00" {
		return header{}, errors.New("not an SQLite database")
	}

	// The encoded value 1 represents a 65536-byte page.
	h := header{pageSize: uint32(binary.BigEndian.Uint16(b[16:18]))}
	if h.pageSize == 1 {
		h.pageSize = 65536
	}
	if h.pageSize < 512 || h.pageSize > 65536 || h.pageSize&(h.pageSize-1) != 0 {
		return header{}, errors.Errorf("invalid SQLite page size %d", h.pageSize)
	}

	// The one-byte reserve leaves at least 257 usable bytes.
	h.usable = h.pageSize - uint32(b[20])

	// Decode the page counts and the freelist head.
	change := binary.BigEndian.Uint32(b[24:28])
	h.dbPages = binary.BigEndian.Uint32(b[28:32])
	validFor := binary.BigEndian.Uint32(b[92:96])
	h.trunk = binary.BigEndian.Uint32(b[32:36])
	h.freeCount = binary.BigEndian.Uint32(b[36:40])
	h.autoVac = binary.BigEndian.Uint32(b[52:56]) != 0

	// Check that the counts still describe the file.
	if validFor != change {
		return h, errors.Errorf("header page count is stale: version-valid-for %d, change counter %d", validFor, change)
	}
	if fileSize != h.pageBytes(h.dbPages) {
		return h, errors.Errorf("file is %d bytes, header says %d pages of %d", fileSize, h.dbPages, h.pageSize)
	}
	return h, nil
}

// bitset is one bit per page number.
type bitset []uint64

func newBitset(n uint32) bitset    { return make(bitset, n/64+1) }
func (s bitset) has(p uint32) bool { return s[p/64]&(1<<(p%64)) != 0 }
func (s bitset) set(p uint32)      { s[p/64] |= 1 << (p % 64) }
func (s bitset) clear(p uint32)    { s[p/64] &^= 1 << (p % 64) }

// walk is one pass over the freelist trunk chain.
type walk struct {
	trunks   []uint32
	leaves   []uint32 // leaves of all trunks, in chain order
	perTrunk []uint32 // leaf count of each trunk
	torn     error    // set when a tolerant walk stopped early
}

// pageCount returns the total number of distinct pages in the walk.
func (w *walk) pageCount() int64 { return int64(len(w.trunks)) + int64(len(w.leaves)) }

// walkFreelist walks the trunk chain. A strict walk returns the first fault
// as an error. A tolerant walk records the fault in torn and returns what
// the walk read.
func walkFreelist(r io.ReaderAt, h header, tolerant bool) (*walk, error) {
	// Size the freelist reader from the validated header.
	w := &walk{}
	seen := newBitset(h.dbPages)
	lockPage := h.lockPage()
	maxLeaves := h.usable/4 - 2
	page := make([]byte, h.pageSize)

	// A tolerant walk returns all complete trunks preceding the fault.
	fail := func(err error) (*walk, error) {
		if tolerant {
			w.torn = err
			return w, nil
		}
		return nil, err
	}

	// Each page must be in range and appear only once.
	check := func(p uint32, what string) error {
		switch {
		case p < 2 || p > h.dbPages:
			return errors.Errorf("%s page %d is outside 2..%d", what, p, h.dbPages)
		case p == lockPage:
			return errors.Errorf("%s page %d is the lock-byte page", what, p)
		case seen.has(p):
			return errors.Errorf("%s page %d is listed twice", what, p)
		}
		seen.set(p)
		return nil
	}

	// Follow the chain and keep each trunk only after its leaves validate.
	for t := h.trunk; t != 0; {
		if err := check(t, "trunk"); err != nil {
			return fail(err)
		}
		if _, err := r.ReadAt(page, h.pageBytes(t-1)); err != nil {
			return fail(errors.Wrapf(err, "read trunk %d", t))
		}
		next := binary.BigEndian.Uint32(page[0:4])
		n := binary.BigEndian.Uint32(page[4:8])
		if n > maxLeaves {
			return fail(errors.Errorf("trunk %d lists %d leaves, at most %d fit", t, n, maxLeaves))
		}
		start := len(w.leaves)
		for i := range n {
			l := binary.BigEndian.Uint32(page[8+4*i:])
			if err := check(l, "leaf"); err != nil {
				w.leaves = w.leaves[:start]
				return fail(errors.Wrapf(err, "trunk %d", t))
			}
			w.leaves = append(w.leaves, l)
		}
		w.trunks = append(w.trunks, t)
		w.perTrunk = append(w.perTrunk, n)
		if w.pageCount() > int64(h.dbPages) {
			return fail(errors.New("freelist is longer than the file"))
		}
		t = next
	}
	return w, nil
}

// pageRange is count pages starting at page start.
type pageRange struct{ start, count uint32 }

// plan is what a punch releases.
type plan struct {
	ranges     []pageRange
	punchPages uint32
	keptTrunks int
	keptPages  uint32
	sha        string
}

// makePlan keeps back the leaves of the head trunks until reserve bytes are
// kept, since SQLite allocates from the head trunk, then coalesces the other
// leaves into ranges in page order. Trunks are never in a range.
func makePlan(w *walk, h header, reserve int64) (plan, error) {
	// Keep whole trunks' leaves until the reserve is met.
	var p plan
	punch := newBitset(h.dbPages)
	var off uint32
	for i, n := range w.perTrunk {
		leaves := w.leaves[off : off+n]
		off += n
		if h.pageBytes(p.keptPages) < reserve {
			p.keptTrunks = i + 1
			p.keptPages += n
			continue
		}
		for _, l := range leaves {
			punch.set(l)
		}
	}

	// Preserve freelist trunks, including their links to the leaves.
	for _, t := range w.trunks {
		punch[t/64] &^= 1 << (t % 64)
	}

	// Coalesce adjacent leaves in ascending page order.
	var wi uint32
	for _, word := range punch {
		for word != 0 {
			b, err := checkedUint32(int64(bits.TrailingZeros64(word)))
			if err != nil {
				return plan{}, errors.Wrap(err, "freelist bit position")
			}
			word &= word - 1
			pg := wi*64 + b
			if n := len(p.ranges); n > 0 && pg-p.ranges[n-1].start == p.ranges[n-1].count {
				p.ranges[n-1].count++
			} else {
				p.ranges = append(p.ranges, pageRange{pg, 1})
			}
			p.punchPages++
		}
		wi++
	}

	// Hash the plan before the caller offers it for punching.
	sha, err := rangeSHA(h, p.ranges, w.trunks, w.leaves[:p.keptPages])
	if err != nil {
		return plan{}, err
	}
	p.sha = sha
	return p, nil
}

// rangeSHA hashes the page size, page count, every range, the trunk chain
// in order and the kept-back leaves, each list prefixed by its length.
func rangeSHA(h header, rs []pageRange, trunks, kept []uint32) (string, error) {
	// Encode each field as a big-endian uint32 to preserve existing hashes.
	d := sha256.New()
	put := func(v ...uint32) {
		var b [4]byte
		for _, x := range v {
			binary.BigEndian.PutUint32(b[:], x)
			d.Write(b[:])
		}
	}
	putLen := func(n int) error {
		count, err := checkedUint32(int64(n))
		if err != nil {
			return err
		}
		put(count)
		return nil
	}

	// Hash the database dimensions and the ranges.
	put(h.pageSize, h.dbPages)
	if err := putLen(len(rs)); err != nil {
		return "", errors.Wrap(err, "hash range count")
	}
	for _, r := range rs {
		put(r.start, r.count)
	}

	// Hash the ordered trunk chain followed by the kept leaves.
	for _, pages := range [][]uint32{trunks, kept} {
		if err := putLen(len(pages)); err != nil {
			return "", errors.Wrap(err, "hash freelist page count")
		}
		put(pages...)
	}
	return hex.EncodeToString(d.Sum(nil)), nil
}

// checkedUint32 converts n if it fits in an unsigned 32-bit field.
func checkedUint32(n int64) (uint32, error) {
	if n < 0 || n > math.MaxUint32 {
		return 0, errors.Errorf("%d is outside the uint32 range", n)
	}
	return uint32(n), nil
}

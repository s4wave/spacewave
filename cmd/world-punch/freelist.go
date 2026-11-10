package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math/bits"

	"github.com/pkg/errors"
)

// pendingByte is SQLite's PENDING_BYTE. The page holding it is the lock-byte
// page, which SQLite never uses and never puts on the freelist.
const pendingByte = 0x40000000

// header is the part of the SQLite database header the punch needs.
type header struct {
	pageSize  int64
	usable    int64
	dbPages   uint32
	trunk     uint32
	freeCount uint32
	autoVac   bool
}

// readHeader reads and checks the 100 byte header of page 1.
func readHeader(r io.ReaderAt, fileSize int64) (header, error) {
	var b [100]byte
	if _, err := r.ReadAt(b[:], 0); err != nil {
		return header{}, errors.Wrap(err, "read header")
	}
	if string(b[:16]) != "SQLite format 3\x00" {
		return header{}, errors.New("not an SQLite database")
	}
	h := header{pageSize: int64(binary.BigEndian.Uint16(b[16:18]))}
	if h.pageSize == 1 {
		h.pageSize = 65536
	}
	h.usable = h.pageSize - int64(b[20])
	change := binary.BigEndian.Uint32(b[24:28])
	h.dbPages = binary.BigEndian.Uint32(b[28:32])
	validFor := binary.BigEndian.Uint32(b[92:96])
	h.trunk = binary.BigEndian.Uint32(b[32:36])
	h.freeCount = binary.BigEndian.Uint32(b[36:40])
	h.autoVac = binary.BigEndian.Uint32(b[52:56]) != 0
	if validFor != change {
		return h, errors.Errorf("header page count is stale: version-valid-for %d, change counter %d", validFor, change)
	}
	if fileSize != int64(h.dbPages)*h.pageSize {
		return h, errors.Errorf("file is %d bytes, header says %d pages of %d", fileSize, h.dbPages, h.pageSize)
	}
	return h, nil
}

// bitset is one bit per page number.
type bitset []uint64

func newBitset(n uint32) bitset    { return make(bitset, n/64+1) }
func (s bitset) has(p uint32) bool { return s[p/64]&(1<<(p%64)) != 0 }
func (s bitset) set(p uint32)      { s[p/64] |= 1 << (p % 64) }

// walk is one pass over the freelist trunk chain.
type walk struct {
	trunks   []uint32
	leaves   []uint32 // leaves of all trunks, in chain order
	perTrunk []int    // leaf count of each trunk
	torn     error    // set when a tolerant walk stopped early
}

// walkFreelist walks the trunk chain. A strict walk returns the first fault
// as an error; a tolerant walk records it in torn and returns what it read.
func walkFreelist(r io.ReaderAt, h header, tolerant bool) (*walk, error) {
	w := &walk{}
	seen := newBitset(h.dbPages)
	lockPage := uint32(pendingByte/h.pageSize + 1)
	maxLeaves := uint32(h.usable/4 - 2)
	page := make([]byte, h.pageSize)
	fail := func(err error) (*walk, error) {
		if tolerant {
			w.torn = err
			return w, nil
		}
		return nil, err
	}
	check := func(p uint32, what string) error {
		switch {
		case p < 2 || p > h.dbPages:
			return errors.Errorf("%s page %d is outside 2..%d", what, p, h.dbPages)
		case p == lockPage && int64(h.dbPages) >= int64(lockPage):
			return errors.Errorf("%s page %d is the lock-byte page", what, p)
		case seen.has(p):
			return errors.Errorf("%s page %d is listed twice", what, p)
		}
		seen.set(p)
		return nil
	}
	for t := h.trunk; t != 0; {
		if err := check(t, "trunk"); err != nil {
			return fail(err)
		}
		if _, err := r.ReadAt(page, int64(t-1)*h.pageSize); err != nil {
			return fail(errors.Wrapf(err, "read trunk %d", t))
		}
		next := binary.BigEndian.Uint32(page[0:4])
		n := binary.BigEndian.Uint32(page[4:8])
		if n > maxLeaves {
			return fail(errors.Errorf("trunk %d lists %d leaves, at most %d fit", t, n, maxLeaves))
		}
		start := len(w.leaves)
		for i := uint32(0); i < n; i++ {
			l := binary.BigEndian.Uint32(page[8+4*i:])
			if err := check(l, "leaf"); err != nil {
				w.leaves = w.leaves[:start]
				return fail(errors.Wrapf(err, "trunk %d", t))
			}
			w.leaves = append(w.leaves, l)
		}
		w.trunks = append(w.trunks, t)
		w.perTrunk = append(w.perTrunk, int(n))
		if uint64(len(w.trunks))+uint64(len(w.leaves)) > uint64(h.dbPages) {
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
	punchPages uint64
	keptTrunks int
	keptPages  uint64
	sha        string
}

// makePlan keeps back the leaves of the head trunks until reserve bytes are
// kept, since SQLite allocates from the head trunk, then coalesces the other
// leaves into ranges in page order. Trunks are never in a range.
func makePlan(w *walk, h header, reserve int64) plan {
	var p plan
	punch := newBitset(h.dbPages)
	off := 0
	for i, n := range w.perTrunk {
		leaves := w.leaves[off : off+n]
		off += n
		if int64(p.keptPages)*h.pageSize < reserve {
			p.keptTrunks = i + 1
			p.keptPages += uint64(n)
			continue
		}
		for _, l := range leaves {
			punch.set(l)
		}
	}
	for _, t := range w.trunks {
		punch[t/64] &^= 1 << (t % 64)
	}
	for wi, word := range punch {
		for word != 0 {
			b := uint32(bits.TrailingZeros64(word))
			word &= word - 1
			pg := uint32(wi)*64 + b
			if n := len(p.ranges); n > 0 && p.ranges[n-1].start+p.ranges[n-1].count == pg {
				p.ranges[n-1].count++
			} else {
				p.ranges = append(p.ranges, pageRange{pg, 1})
			}
			p.punchPages++
		}
	}
	p.sha = rangeSHA(h, p.ranges)
	return p
}

// rangeSHA hashes the page size, page count and every range.
func rangeSHA(h header, rs []pageRange) string {
	d := sha256.New()
	var b [8]byte
	binary.BigEndian.PutUint32(b[0:4], uint32(h.pageSize))
	binary.BigEndian.PutUint32(b[4:8], h.dbPages)
	d.Write(b[:])
	for _, r := range rs {
		binary.BigEndian.PutUint32(b[0:4], r.start)
		binary.BigEndian.PutUint32(b[4:8], r.count)
		d.Write(b[:])
	}
	return hex.EncodeToString(d.Sum(nil))
}

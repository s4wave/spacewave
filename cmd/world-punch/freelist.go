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

// lockByteOffset is where crosscheck and scan place the lock-byte page; a
// test moves it into a small file.
var lockByteOffset int64 = pendingByte

// lockBytePage is the page crosscheck and scan treat as the lock-byte page.
func lockBytePage(h header) uint32 { return uint32(lockByteOffset/h.pageSize + 1) }

// header is the part of the SQLite database header the punch needs.
type header struct {
	pageSize  int64
	usable    int64
	dbPages   uint32
	trunk     uint32
	freeCount uint32
	change    uint32 // file change counter at offset 24
	autoVac   bool
}

// readHeader reads and checks the 100 byte header of page 1.
func readHeader(r io.ReaderAt, fileSize int64) (header, error) {
	// Read the header and check the magic string.
	var b [100]byte
	if _, err := r.ReadAt(b[:], 0); err != nil {
		return header{}, errors.Wrap(err, "read header")
	}
	if string(b[:16]) != "SQLite format 3\x00" {
		return header{}, errors.New("not an SQLite database")
	}

	// Refuse a page size or usable size SQLite would not write.
	h := header{pageSize: int64(binary.BigEndian.Uint16(b[16:18]))}
	if h.pageSize == 1 {
		h.pageSize = 65536
	}
	h.usable = h.pageSize - int64(b[20])
	if h.pageSize < 512 || h.pageSize > 65536 || h.pageSize&(h.pageSize-1) != 0 {
		return header{}, errors.Errorf("header page size %d: want a power of two from 512 to 65536", h.pageSize)
	}
	if h.usable < 480 {
		return header{}, errors.Errorf("header usable size %d (page size %d, reserved %d): want at least 480", h.usable, h.pageSize, b[20])
	}

	// Take the counts and the freelist head.
	h.change = binary.BigEndian.Uint32(b[24:28])
	h.dbPages = binary.BigEndian.Uint32(b[28:32])
	validFor := binary.BigEndian.Uint32(b[92:96])
	h.trunk = binary.BigEndian.Uint32(b[32:36])
	h.freeCount = binary.BigEndian.Uint32(b[36:40])
	h.autoVac = binary.BigEndian.Uint32(b[52:56]) != 0

	// Warn of a stale page count or a file size unlike it.
	if validFor != h.change {
		return h, errors.Errorf("header page count is stale: version-valid-for %d, change counter %d", validFor, h.change)
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
func (s bitset) clear(p uint32)    { s[p/64] &^= 1 << (p % 64) }

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
	p.sha = rangeSHA(h, p.ranges, w.trunks, w.leaves[:p.keptPages])
	return p
}

// rangeSHA hashes the page size, page count, every range, the trunk chain
// in order and the kept-back leaves, each list prefixed by its length.
func rangeSHA(h header, rs []pageRange, trunks, kept []uint32) string {
	d := sha256.New()
	put := func(v ...uint32) {
		var b [4]byte
		for _, x := range v {
			binary.BigEndian.PutUint32(b[:], x)
			d.Write(b[:])
		}
	}
	put(uint32(h.pageSize), h.dbPages, uint32(len(rs)))
	for _, r := range rs {
		put(r.start, r.count)
	}
	put(uint32(len(trunks)))
	put(trunks...)
	put(uint32(len(kept)))
	put(kept...)
	return hex.EncodeToString(d.Sum(nil))
}

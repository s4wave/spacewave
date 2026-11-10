// Package sqlitefree reads the freelist of an SQLite database file and plans
// which of its leaf pages to release from the disk with
// fallocate(PUNCH_HOLE|KEEP_SIZE). cmd/world-punch runs it on a stopped file;
// the sqlite kvtx store runs it under its writer turn and SQLite's write lock.
//
// A free leaf page holds no content SQLite reads: it allocates a leaf it takes
// from the freelist without reading it, as a zeroed page. Trunk pages hold the
// chain and are never released.
package sqlitefree

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math/bits"

	"github.com/pkg/errors"
)

// PendingByte is SQLite's PENDING_BYTE. The page holding it is the lock-byte
// page, which SQLite never uses and never puts on the freelist.
const PendingByte = 0x40000000

// Header is the part of the SQLite database header the punch needs.
type Header struct {
	PageSize  int64
	Usable    int64
	DBPages   uint32
	Trunk     uint32
	FreeCount uint32
	Change    uint32 // file change counter at offset 24
	AutoVac   bool
}

// ReadHeader reads and checks the 100 byte header of page 1.
func ReadHeader(r io.ReaderAt, fileSize int64) (Header, error) {
	// Read the header and check the magic string.
	var b [100]byte
	if _, err := r.ReadAt(b[:], 0); err != nil {
		return Header{}, errors.Wrap(err, "read header")
	}
	if string(b[:16]) != "SQLite format 3\x00" {
		return Header{}, errors.New("not an SQLite database")
	}

	// Refuse a page size or usable size SQLite would not write.
	h := Header{PageSize: int64(binary.BigEndian.Uint16(b[16:18]))}
	if h.PageSize == 1 {
		h.PageSize = 65536
	}
	h.Usable = h.PageSize - int64(b[20])
	if h.PageSize < 512 || h.PageSize > 65536 || h.PageSize&(h.PageSize-1) != 0 {
		return Header{}, errors.Errorf("header page size %d: want a power of two from 512 to 65536", h.PageSize)
	}
	if h.Usable < 480 {
		return Header{}, errors.Errorf("header usable size %d (page size %d, reserved %d): want at least 480", h.Usable, h.PageSize, b[20])
	}

	// Take the counts and the freelist head.
	h.Change = binary.BigEndian.Uint32(b[24:28])
	h.DBPages = binary.BigEndian.Uint32(b[28:32])
	validFor := binary.BigEndian.Uint32(b[92:96])
	h.Trunk = binary.BigEndian.Uint32(b[32:36])
	h.FreeCount = binary.BigEndian.Uint32(b[36:40])
	h.AutoVac = binary.BigEndian.Uint32(b[52:56]) != 0

	// Warn of a stale page count or a file size unlike it.
	if validFor != h.Change {
		return h, errors.Errorf("header page count is stale: version-valid-for %d, change counter %d", validFor, h.Change)
	}
	if fileSize != int64(h.DBPages)*h.PageSize {
		return h, errors.Errorf("file is %d bytes, header says %d pages of %d", fileSize, h.DBPages, h.PageSize)
	}
	return h, nil
}

// Bitset is one bit per page number.
type Bitset []uint64

// NewBitset returns a Bitset for pages 0 to n.
func NewBitset(n uint32) Bitset { return make(Bitset, n/64+1) }

// Has reports whether page p is set.
func (s Bitset) Has(p uint32) bool { return s[p/64]&(1<<(p%64)) != 0 }

// Set sets page p.
func (s Bitset) Set(p uint32) { s[p/64] |= 1 << (p % 64) }

// Clear clears page p.
func (s Bitset) Clear(p uint32) { s[p/64] &^= 1 << (p % 64) }

// Walk is one pass over the freelist trunk chain.
type Walk struct {
	Trunks   []uint32
	Leaves   []uint32 // leaves of all trunks, in chain order
	PerTrunk []int    // leaf count of each trunk
	Torn     error    // set when a tolerant walk stopped early
}

// WalkFreelist walks the trunk chain. A strict walk returns the first fault
// as an error; a tolerant walk records it in Torn and returns what it read.
func WalkFreelist(r io.ReaderAt, h Header, tolerant bool) (*Walk, error) {
	// Size the walk's buffers and limits to the header.
	w := &Walk{}
	seen := NewBitset(h.DBPages)
	lockPage := uint32(PendingByte/h.PageSize + 1) //nolint:gosec // page size is at least 512.
	maxLeaves := uint32(h.Usable/4 - 2)            //nolint:gosec // usable is 480 to 65536.
	page := make([]byte, h.PageSize)

	// A fault ends a strict walk and is kept by a tolerant one.
	fail := func(err error) (*Walk, error) {
		if tolerant {
			w.Torn = err
			return w, nil
		}
		return nil, err
	}

	// Refuse a page outside the file, the lock-byte page, or one seen before.
	check := func(p uint32, what string) error {
		switch {
		case p < 2 || p > h.DBPages:
			return errors.Errorf("%s page %d is outside 2..%d", what, p, h.DBPages)
		case p == lockPage && int64(h.DBPages) >= int64(lockPage):
			return errors.Errorf("%s page %d is the lock-byte page", what, p)
		case seen.Has(p):
			return errors.Errorf("%s page %d is listed twice", what, p)
		}
		seen.Set(p)
		return nil
	}

	// Follow the chain from the header's first trunk, taking each trunk's
	// leaves in order.
	for t := h.Trunk; t != 0; {
		if err := check(t, "trunk"); err != nil {
			return fail(err)
		}
		if _, err := r.ReadAt(page, int64(t-1)*h.PageSize); err != nil {
			return fail(errors.Wrapf(err, "read trunk %d", t))
		}
		next := binary.BigEndian.Uint32(page[0:4])
		n := binary.BigEndian.Uint32(page[4:8])
		if n > maxLeaves {
			return fail(errors.Errorf("trunk %d lists %d leaves, at most %d fit", t, n, maxLeaves))
		}
		start := len(w.Leaves)
		for i := uint32(0); i < n; i++ {
			l := binary.BigEndian.Uint32(page[8+4*i:])
			if err := check(l, "leaf"); err != nil {
				w.Leaves = w.Leaves[:start]
				return fail(errors.Wrapf(err, "trunk %d", t))
			}
			w.Leaves = append(w.Leaves, l)
		}
		w.Trunks = append(w.Trunks, t)
		w.PerTrunk = append(w.PerTrunk, int(n))
		if uint64(len(w.Trunks))+uint64(len(w.Leaves)) > uint64(h.DBPages) {
			return fail(errors.New("freelist is longer than the file"))
		}
		t = next
	}
	return w, nil
}

// Range is count pages starting at page start.
type Range struct{ Start, Count uint32 }

// Plan is what a punch releases.
type Plan struct {
	Ranges     []Range
	PunchPages uint64
	KeptTrunks int
	KeptPages  uint64
	SHA        string
}

// MakePlan keeps back the leaves of the head trunks until reserve bytes are
// kept, since SQLite allocates from the head trunk, then coalesces the other
// leaves into ranges in page order. Trunks are never in a range.
func MakePlan(w *Walk, h Header, reserve int64) Plan {
	// Keep the head trunks' leaves up to the reserve and mark the rest.
	var p Plan
	punch := NewBitset(h.DBPages)
	off := 0
	for i, n := range w.PerTrunk {
		leaves := w.Leaves[off : off+n]
		off += n
		if int64(p.KeptPages)*h.PageSize < reserve { //nolint:gosec // pages fit a uint32.
			p.KeptTrunks = i + 1
			p.KeptPages += uint64(n) //nolint:gosec // n is not negative.
			continue
		}
		for _, l := range leaves {
			punch.Set(l)
		}
	}

	// Never release a trunk.
	for _, t := range w.Trunks {
		punch[t/64] &^= 1 << (t % 64)
	}

	// Coalesce the marked pages into ranges in page order.
	for wi, word := range punch {
		for word != 0 {
			b := uint32(bits.TrailingZeros64(word)) //nolint:gosec // at most 64.
			word &= word - 1
			pg := uint32(wi)*64 + b
			if n := len(p.Ranges); n > 0 && p.Ranges[n-1].Start+p.Ranges[n-1].Count == pg {
				p.Ranges[n-1].Count++
			} else {
				p.Ranges = append(p.Ranges, Range{pg, 1})
			}
			p.PunchPages++
		}
	}

	// Hash the plan so a later walk can be matched against it.
	p.SHA = RangeSHA(h, p.Ranges, w.Trunks, w.Leaves[:p.KeptPages])
	return p
}

// RangeSHA hashes the page size, page count, every range, the trunk chain
// in order and the kept-back leaves, each list prefixed by its length.
func RangeSHA(h Header, rs []Range, trunks, kept []uint32) string {
	// Write each value big-endian into the hash.
	d := sha256.New()
	put := func(v ...uint32) {
		var b [4]byte
		for _, x := range v {
			binary.BigEndian.PutUint32(b[:], x)
			d.Write(b[:])
		}
	}

	// Hash the file shape, the ranges, the chain and the kept leaves.
	put(uint32(h.PageSize), h.DBPages, uint32(len(rs))) //nolint:gosec // lengths are page counts.
	for _, r := range rs {
		put(r.Start, r.Count)
	}
	put(uint32(len(trunks))) //nolint:gosec // a page count.
	put(trunks...)
	put(uint32(len(kept))) //nolint:gosec // a page count.
	put(kept...)
	return hex.EncodeToString(d.Sum(nil))
}

// StrictPlan walks the chain strictly and plans the punch, refusing unless
// trunks plus leaves equals the header's freelist count.
func StrictPlan(r io.ReaderAt, h Header, reserve int64) (*Walk, Plan, error) {
	// Walk the chain, refusing at its first fault.
	w, err := WalkFreelist(r, h, false)
	if err != nil {
		return nil, Plan{}, err
	}

	// Refuse a chain whose length the header does not count.
	if got := uint64(len(w.Trunks)) + uint64(len(w.Leaves)); got != uint64(h.FreeCount) {
		return nil, Plan{}, errors.Errorf("trunks %d + leaves %d = %d, header freelist count is %d", len(w.Trunks), len(w.Leaves), got, h.FreeCount)
	}
	return w, MakePlan(w, h, reserve), nil
}

// CheckAutoVacuum refuses a file with auto_vacuum on, whose pointer-map pages
// record free pages and whose freelist SQLite moves on its own.
func CheckAutoVacuum(h Header) error {
	if h.AutoVac {
		return errors.New("auto_vacuum is not NONE (header offset 52 is nonzero): ptrmap pages track free pages, refusing")
	}
	return nil
}

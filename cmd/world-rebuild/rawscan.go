package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
	"unsafe"
)

// rawFile reads the pages of a SQLite file directly, without SQLite and so
// without any lock: a reader of a live file never holds its checkpointer. It
// sees the main file as of the last checkpoint, not the WAL, and a page a
// checkpoint is writing may read torn, so its results are a snapshot estimate
// and every decode is bounds checked.
type rawFile struct {
	f        *os.File
	path     string
	pageSize int
	usable   int
	pages    int64
	// free marks the freelist's pages, whose stale contents are skipped.
	free []uint64
	// direct is set when reads bypass the page cache.
	direct bool
}

// alignedBuf returns n bytes aligned to 4096, as O_DIRECT needs.
func alignedBuf(n int) []byte {
	b := make([]byte, n+4096)
	off := int(uintptr(unsafe.Pointer(&b[0])) & 4095)
	if off != 0 {
		off = 4096 - off
	}
	return b[off : off+n]
}

func openRaw(path string) (*rawFile, error) {
	r := &rawFile{path: path}
	f, err := os.OpenFile(path, os.O_RDONLY|oDirect, 0)
	if err == nil && oDirect != 0 {
		r.direct = true
	} else {
		f, err = os.Open(path)
	}
	if err != nil {
		return nil, err
	}
	r.f = f
	hdr := alignedBuf(4096)
	if _, err := f.ReadAt(hdr, 0); err != nil && !errors.Is(err, io.EOF) {
		f.Close()
		return nil, err
	}
	if string(hdr[:16]) != "SQLite format 3\x00" {
		f.Close()
		return nil, fmt.Errorf("%s: not a SQLite file", path)
	}
	r.pageSize = int(binary.BigEndian.Uint16(hdr[16:]))
	if r.pageSize == 1 {
		r.pageSize = 65536
	}
	r.usable = r.pageSize - int(hdr[20])
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	r.pages = info.Size() / int64(r.pageSize)
	if hdr[100] != 0x0d {
		f.Close()
		return nil, fmt.Errorf("%s: the schema spans more than page 1; a raw scan would read its rows", path)
	}
	r.free = make([]uint64, r.pages/64+1)
	if err := r.loadFreelist(binary.BigEndian.Uint32(hdr[32:]), binary.BigEndian.Uint32(hdr[36:])); err != nil {
		f.Close()
		return nil, err
	}
	return r, nil
}

func (r *rawFile) Close() error { return r.f.Close() }

func (r *rawFile) markFree(pg int64) {
	if pg >= 1 && pg <= r.pages {
		r.free[pg/64] |= 1 << (pg % 64)
	}
}

func (r *rawFile) isFree(pg int64) bool {
	return r.free[pg/64]&(1<<(pg%64)) != 0
}

// page reads one page by its 1-based number.
func (r *rawFile) page(pg int64, buf []byte) error {
	_, err := r.f.ReadAt(buf[:r.pageSize], (pg-1)*int64(r.pageSize))
	return err
}

// loadFreelist marks the trunk and leaf pages of the freelist.
func (r *rawFile) loadFreelist(trunk, count uint32) error {
	buf := alignedBuf(r.pageSize)
	var seen uint32
	for pg := int64(trunk); pg != 0; {
		if pg > r.pages || seen > count {
			return fmt.Errorf("%s: freelist trunk %d out of range", r.path, pg)
		}
		if err := r.page(pg, buf); err != nil {
			return err
		}
		r.markFree(pg)
		seen++
		next := int64(binary.BigEndian.Uint32(buf))
		n := int(binary.BigEndian.Uint32(buf[4:]))
		for i := 0; i < n && 8+4*i+4 <= r.usable; i++ {
			r.markFree(int64(binary.BigEndian.Uint32(buf[8+4*i:])))
			seen++
		}
		pg = next
	}
	return nil
}

// rawCell is one table leaf cell.
type rawCell struct {
	rowid int64
	// size is the payload size; local the bytes on the leaf page.
	size  int64
	local []byte
	// overflow is the first overflow page, 0 when the payload is local.
	overflow uint32
}

// varint decodes a SQLite varint.
func varint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < 9; i++ {
		if i >= len(b) {
			return 0, 0
		}
		if i == 8 {
			return v<<8 | uint64(b[i]), 9
		}
		v = v<<7 | uint64(b[i]&0x7f)
		if b[i] < 0x80 {
			return v, i + 1
		}
	}
	return 0, 0
}

// localSize is the payload a table leaf keeps on the page.
func (r *rawFile) localSize(p int64) int64 {
	u := int64(r.usable)
	x := u - 35
	if p <= x {
		return p
	}
	m := (u-12)*32/255 - 23
	k := m + (p-m)%(u-4)
	if k <= x {
		return k
	}
	return m
}

// leafCells calls fn with each cell of a table leaf page, and returns the
// cells it could not decode.
func (r *rawFile) leafCells(pg int64, p []byte, fn func(c rawCell)) int {
	h := 0
	if pg == 1 {
		h = 100
	}
	if len(p) < h+8 || p[h] != 0x0d {
		return 0
	}
	n := int(binary.BigEndian.Uint16(p[h+3:]))
	bad := 0
	for i := range n {
		at := h + 8 + 2*i
		if at+2 > r.usable {
			return bad + n - i
		}
		off := int(binary.BigEndian.Uint16(p[at:]))
		if off < h+8 || off >= r.usable {
			bad++
			continue
		}
		size, k := varint(p[off:r.usable])
		if k == 0 {
			bad++
			continue
		}
		rowid, k2 := varint(p[off+k : r.usable])
		if k2 == 0 {
			bad++
			continue
		}
		start := off + k + k2
		local := r.localSize(int64(size))
		end := start + int(local)
		c := rawCell{rowid: int64(rowid), size: int64(size)}
		if local < int64(size) {
			if end+4 > r.usable {
				bad++
				continue
			}
			c.overflow = binary.BigEndian.Uint32(p[end:])
		}
		if end > r.usable {
			bad++
			continue
		}
		c.local = p[start:end]
		fn(c)
	}
	return bad
}

// kvRecord decodes a two column record whose columns are blobs or NULL: the
// key, which must be on the page, and the value's length and its local bytes.
func kvRecord(c rawCell) (key []byte, vlen int64, vlocal []byte, ok bool) {
	b := c.local
	hs, k := varint(b)
	if k == 0 || int(hs) > len(b) || hs < 3 {
		return nil, 0, nil, false
	}
	t1, k1 := varint(b[k:hs])
	if k1 == 0 {
		return nil, 0, nil, false
	}
	t2, k2 := varint(b[k+k1 : hs])
	if k2 == 0 || uint64(k+k1+k2) != hs {
		return nil, 0, nil, false
	}
	blen := func(t uint64) (int64, bool) {
		switch {
		case t == 0:
			return 0, true
		case t >= 12 && t%2 == 0:
			return int64(t-12) / 2, true
		case t >= 13:
			return int64(t-13) / 2, true
		}
		return 0, false
	}
	kl, ok1 := blen(t1)
	vl, ok2 := blen(t2)
	if !ok1 || !ok2 || int64(hs)+kl > int64(len(b)) || int64(hs)+kl+vl != c.size {
		return nil, 0, nil, false
	}
	key = b[hs : int64(hs)+kl]
	vlocal = b[int64(hs)+kl:]
	return key, vl, vlocal, true
}

// value reads a cell's whole value, following its overflow chain.
func (r *rawFile) value(c rawCell, vlen int64, vlocal []byte) ([]byte, error) {
	v := append([]byte(nil), vlocal...)
	buf := alignedBuf(r.pageSize)
	for pg := int64(c.overflow); pg != 0 && int64(len(v)) < vlen; {
		if pg > r.pages {
			return nil, fmt.Errorf("overflow page %d out of range", pg)
		}
		if err := r.page(pg, buf); err != nil {
			return nil, err
		}
		next := int64(binary.BigEndian.Uint32(buf))
		v = append(v, buf[4:r.usable][:min(int64(r.usable-4), vlen-int64(len(v)))]...)
		pg = next
	}
	if int64(len(v)) != vlen {
		return nil, fmt.Errorf("value of %d bytes, chain gave %d", vlen, len(v))
	}
	return v, nil
}

// rawStats counts a raw pass.
type rawStats struct {
	pages, leaves, free, badCells int64
	read                          int64
	took                          time.Duration
}

// scanLeaves reads the file front to back in chunks, at most mbps megabytes
// a second (0: no limit), and calls fn with each cell of each table leaf
// page not on the freelist. Each cell's local bytes are valid only during fn.
// progress, when set, follows the bytes read.
func (r *rawFile) scanLeaves(mbps float64, chunk int, progress *int64, fn func(c rawCell)) (rawStats, error) {
	return r.scanRange(0, r.pages*int64(r.pageSize), mbps, chunk, progress, fn)
}

// scanRange is scanLeaves over the bytes [from, to) of the file; from must be
// page aligned.
func (r *rawFile) scanRange(from, to int64, mbps float64, chunk int, progress *int64, fn func(c rawCell)) (rawStats, error) {
	// Size the read buffer to whole pages and clamp the range to the file.
	var st rawStats
	start := time.Now()
	chunk -= chunk % r.pageSize
	buf := alignedBuf(chunk)
	size := min(to, r.pages*int64(r.pageSize))

	// Read chunk by chunk, decode the leaf pages, then pace.
	for off := from; off < size; {
		n := int(min(int64(chunk), size-off))
		got, err := r.f.ReadAt(buf[:n], off)
		if err != nil && !errors.Is(err, io.EOF) {
			return st, err
		}
		got -= got % r.pageSize
		for i := 0; i < got; i += r.pageSize {
			pg := (off+int64(i))/int64(r.pageSize) + 1
			st.pages++
			if r.isFree(pg) {
				st.free++
				continue
			}
			p := buf[i : i+r.pageSize]
			// Page 1 roots the schema table, whose rows are not the
			// key-value table's; openRaw checks it is a single leaf.
			if pg == 1 || p[0] != 0x0d {
				continue
			}
			st.leaves++
			st.badCells += int64(r.leafCells(pg, p, fn))
		}
		off += int64(got)
		st.read += int64(got)
		if progress != nil {
			*progress = st.read
		}
		if got == 0 {
			break
		}
		if mbps > 0 {
			want := time.Duration(float64(st.read) / (mbps * 1e6) * float64(time.Second))
			if el := time.Since(start); el < want {
				time.Sleep(want - el)
			}
		}
		if live != nil {
			live.done(0, 0)
		}
	}

	// Record the pass time.
	st.took = time.Since(start)
	return st, nil
}

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// scanChunk is the size of one phase 2 read.
const scanChunk = 1 << 20

// chainHead is the first page of an overflow chain, the page count its
// payload implies, and the b-tree page whose cell holds it.
type chainHead struct {
	pg, pages, parent uint32
}

// scanStream is what one phase 2 stream found in its chunks.
type scanStream struct {
	heads  []chainHead
	bad    []badPage
	kinds  [numKinds]uint64
	byType [256]uint64
	cells  uint64
	pages  int64
}

// scanCheck checks a stopped file like crosscheck, but reads the leaves in
// one sequential pass. Phase 1 walks the interior pages with random reads
// and all of sqlite_schema; phase 2 reads the file in page order, parsing
// leaf pages and keeping every other page's first word; phase 3 follows
// the overflow chains through those words in memory.
func scanCheck(f io.ReaderAt, h header, o ccOpts) (ccResult, error) {
	// Size the result against the header.
	start := time.Now()
	res := ccResult{dbPages: h.dbPages, byType: map[byte]uint64{}, startFree: h.freeCount, inUse: h.dbPages, strict: true, startChange: h.change}
	if h.freeCount < h.dbPages {
		res.inUse = h.dbPages - h.freeCount
	}
	o.readers, o.streams = max(o.readers, 1), max(o.streams, 1)
	bulk := o.bulk
	if bulk == nil {
		bulk = f
	}

	// Walk the freelist once; the file is stopped, so once is enough.
	free, fw := freeSet(f, h)
	res.freeTorn = fw.torn
	res.freeTrunks, res.freeLeaves = uint64(len(fw.trunks)), uint64(len(fw.leaves))
	if o.freelistOverride != nil {
		o.freelistOverride(free, false)
	}

	// Count reads and problems for the progress line.
	var phase, p2start, readPages, reads, scanned, nConf, nDup, nBad atomic.Int64
	addConflict := func(c conflict) {
		res.conflicts = append(res.conflicts, c)
		nConf.Add(1)
		if c.dup {
			nDup.Add(1)
		}
	}
	addBad := func(b badPage) {
		res.bad = append(res.bad, b)
		nBad.Add(1)
	}

	// Print progress until the check returns.
	stop := make(chan struct{})
	defer close(stop)
	if o.every > 0 && o.out != nil {
		go func() {
			tk := time.NewTicker(o.every)
			defer tk.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tk.C:
					scanProgress(o.out, time.Since(time.Unix(0, max(p2start.Load(), start.UnixNano()))), h, res.inUse, phase.Load(), readPages.Load(), reads.Load(), scanned.Load(), nConf.Load(), nDup.Load(), nBad.Load())
				}
			}
		}()
	}

	// mark claims a page for the walk: out of range is bad, reached
	// before is a dup, and on the freelist conflicts.
	reached, btree, leaf := newBitset(h.dbPages), newBitset(h.dbPages), newBitset(h.dbPages)
	mark := func(it ccItem) bool {
		// Refuse an out-of-range or already reached page.
		if it.pg < 1 || it.pg > h.dbPages {
			addBad(badPage{page: it.parent, err: fmt.Sprintf("points to page %d outside 1..%d", it.pg, h.dbPages), ptr: true})
			return false
		}
		if it.pg == lockBytePage(h) {
			addBad(badPage{page: it.parent, err: fmt.Sprintf("points to the lock-byte page %d", it.pg), ptr: true})
			return false
		}
		if reached.has(it.pg) {
			addConflict(conflict{page: it.pg, parent: it.parent, kind: it.kind, dup: true, parent2: it.parent})
			return false
		}

		// Claim it; a free one conflicts.
		reached.set(it.pg)
		if free.has(it.pg) {
			addConflict(conflict{page: it.pg, parent: it.parent, kind: it.kind})
		}
		return true
	}

	// Phase 1: read interior pages and all of sqlite_schema. Of the
	// children of an interior page only the first is read, as a probe;
	// the tree is balanced, so if it is a leaf its siblings are leaves.
	lim := &limiter{rate: o.mbps * 1e6}
	var heads []chainHead
	probes := map[uint32][]ccItem{}
	probeStop := make(chan struct{})
	interrupted := func() bool {
		select {
		case <-o.interrupt:
			return true
		case <-probeStop:
			return true
		default:
			return false
		}
	}
	phase.Store(1)
	var queue []ccItem
	if mark(ccItem{pg: 1, kind: kindTree, schema: true}) {
		queue = append(queue, ccItem{pg: 1, kind: kindTree, schema: true})
	}

	// Read round by round until no interior candidate is left.
	for len(queue) > 0 && !interrupted() {
		// Read the round in page order.
		res.rounds++
		slices.SortFunc(queue, func(a, b ccItem) int { return int(a.pg) - int(b.pg) })
		results, err := readRound(bulk, h, queue, o.readers, lim, o.interrupt, func(n int) {
			readPages.Add(int64(n))
			reads.Add(1)
		})
		if err != nil {
			return res, err
		}

		// Sort each page into the tree or leaf set and queue what to read next.
		var next []ccItem
		for _, r := range results {
			next = scanPhase1(&res, r, probes, btree, leaf, &heads, mark, addBad, next)
		}
		queue = next
	}

	// Stop with the totals so far if interrupted.
	res.phaseTimes[0] = time.Since(start)
	finish := func() (ccResult, error) {
		res.readPages, res.reads = readPages.Load()+scanned.Load()/h.pageSize, reads.Load()+res.scanReads
		res.readBytes, res.elapsed, res.interrupted = res.readPages*h.pageSize, time.Since(start), interrupted()
		return res, nil
	}
	if interrupted() {
		res.phase = "phase 1 (interior walk)"
		return finish()
	}

	// Phase 2: read the file in page order from o.streams streams,
	// handing chunks to o.parsers parse workers.
	phase.Store(2)
	t2 := time.Now()
	p2start.Store(t2.UnixNano())
	nextPg := make([]uint32, uint64(h.dbPages)+1)
	total := int64(h.dbPages) * h.pageSize

	// Run the probe beside the pass.
	p2done := make(chan struct{})
	probeFin := make(chan struct{})
	go func() {
		defer close(probeFin)
		scanProbe(o, total, t2, &scanned, p2done, probeStop, &res.probe)
	}()
	outs, err := scanPass(bulk, h, o, lim, leaf, nextPg, total, &scanned, &res.pass, interrupted)
	close(p2done)
	<-probeFin
	if err != nil {
		return res, err
	}

	// Merge the parse workers' finds.
	for i := range outs {
		out := &outs[i]
		heads = append(heads, out.heads...)
		for _, b := range out.bad {
			addBad(b)
		}
		res.leafCells += out.cells
		for k := range out.kinds {
			res.kinds[k] += out.kinds[k]
		}
		for t, n := range out.byType {
			if n > 0 {
				res.byType[byte(t)] += n
			}
		}
	}

	// Stop with the totals so far if interrupted or the probe gave up.
	res.scanReads = res.pass.reads
	res.phaseTimes[1] = time.Since(t2)
	res.scanBytes = scanned.Load()
	if interrupted() {
		res.phase = "phase 2 (sequential scan)"
		if res.probe.stopped {
			res.phase += ": " + res.probe.line
		}
		return finish()
	}

	// Phase 3: follow every overflow chain through the first words.
	phase.Store(3)
	t3 := time.Now()
	for _, hd := range heads {
		scanChain(&res, h, hd, nextPg, reached, btree, leaf, free, addConflict, addBad)
	}

	// Count pages reached, free and neither, as crosscheck does; a
	// reached lock-byte page is bad.
	lockPg := lockBytePage(h)
	for pg := uint32(1); pg <= h.dbPages; pg++ {
		r, fr := reached.has(pg), free.has(pg)
		if r && pg == lockPg {
			addBad(badPage{page: pg, err: "lock-byte page reached"})
		}
		switch {
		case r:
			res.reached++
		case fr:
			res.free++
		case pg == lockPg:
			res.lockPage = 1
		default:
			res.unaccounted++
			res.unaccPages = append(res.unaccPages, pg)
		}
		if r && fr {
			res.free++
		}
	}
	res.phaseTimes[2] = time.Since(t3)

	// Read the header again for the end counts.
	var hb [100]byte
	got, err := f.ReadAt(hb[:], 0)
	if err := fullRead(got, len(hb), 0, err); err != nil {
		return res, fmt.Errorf("scan: end header: %w", err)
	}
	res.endPages = binary.BigEndian.Uint32(hb[28:32])
	res.endFree = binary.BigEndian.Uint32(hb[36:40])
	res.endChange = binary.BigEndian.Uint32(hb[24:28])
	res.leftoverByType = map[string]int{}
	return finish()
}

// scanPhase1 handles one page read in phase 1: a bad page, a leaf found
// by a probe or as a root, or an interior or sqlite_schema page whose
// children and overflow heads it records.
func scanPhase1(res *ccResult, r pageResult, probes map[uint32][]ccItem, btree, leaf bitset, heads *[]chainHead,
	mark func(ccItem) bool, addBad func(badPage), next []ccItem) []ccItem {
	// A bad page has no children; its probe siblings are read instead.
	it := r.item
	sibs := probes[it.pg]
	delete(probes, it.pg)
	if r.bad != "" {
		addBad(badPage{page: it.pg, err: r.bad})
		return append(next, sibs...)
	}

	// A leaf outside sqlite_schema joins the leaf set with its siblings,
	// for phase 2 to parse.
	if (r.ptype == 0x0d || r.ptype == 0x0a) && !it.schema {
		leaf.set(it.pg)
		for _, s := range sibs {
			leaf.set(s.pg)
		}
		return next
	}

	// Count an interior or sqlite_schema page; an interior probe's
	// siblings are interior too, so they are read.
	btree.set(it.pg)
	res.kinds[r.tree]++
	res.byType[r.ptype]++
	res.leafCells += uint64(r.leafCells)
	next = append(next, sibs...)

	// Record overflow heads; read sqlite_schema children and roots, and
	// probe the first child of any other interior page.
	var kids []ccItem
	for _, c := range r.children {
		switch {
		case c.kind == kindOverflow:
			*heads = append(*heads, chainHead{pg: c.pg, pages: c.left, parent: it.pg})
		case !mark(c):
		case it.schema:
			next = append(next, c)
		default:
			kids = append(kids, c)
		}
	}
	if len(kids) > 0 {
		next = append(next, kids[0])
		probes[kids[0].pg] = kids[1:]
	}
	return next
}

// scanPassStats describes phase 2's reads and parse workers.
type scanPassStats struct {
	reads, handoffs, queueFull int64
	parsers                    int
	parsePages                 int64
	parseBusy                  time.Duration // summed over the workers
}

// scanPass reads every chunk from o.streams readers into a fixed set of
// buffers and hands each through a bounded queue to o.parsers parse
// workers, which return the buffer when done. A reader that finds the
// queue full counts it: parsing was then the bottleneck.
func scanPass(f io.ReaderAt, h header, o ccOpts, lim *limiter, leaf bitset, nextPg []uint32, total int64,
	scanned *atomic.Int64, st *scanPassStats, interrupted func() bool) ([]scanStream, error) {
	// Allocate every buffer once.
	type job struct {
		b   []byte
		off int64
	}
	parsers := max(o.parsers, 1)
	st.parsers = parsers
	bufs := make(chan []byte, o.streams+2*parsers)
	for range cap(bufs) {
		bufs <- alignedBuf(scanChunk)
	}
	queue := make(chan job, 2*parsers)

	// Start the parse workers.
	outs := make([]scanStream, parsers)
	busy := make([]time.Duration, parsers)
	var pwg sync.WaitGroup
	for w := range outs {
		pwg.Add(1)
		go func(w int) {
			defer pwg.Done()
			for j := range queue {
				t0 := time.Now()
				parseChunk(h, j.b, j.off, leaf, nextPg, &outs[w])
				busy[w] += time.Since(t0)
				scanned.Add(int64(len(j.b)))
				bufs <- j.b[:cap(j.b)]
			}
		}(w)
	}

	// Start the readers; each takes the next chunk, reads it whole, and
	// queues it.
	nChunks := (total + scanChunk - 1) / scanChunk
	var chunk, reads, handoffs, full atomic.Int64
	var errMu sync.Mutex
	var readErr error
	var rwg sync.WaitGroup
	for range o.streams {
		rwg.Add(1)
		go func() {
			defer rwg.Done()
			for !interrupted() {
				c := chunk.Add(1) - 1
				if c >= nChunks {
					return
				}
				off := c * scanChunk
				b := (<-bufs)[:min(scanChunk, total-off)]
				lim.wait(len(b))
				got, err := f.ReadAt(b, off)
				if err := fullRead(got, len(b), off, err); err != nil {
					errMu.Lock()
					readErr = fmt.Errorf("scan: %w", err)
					errMu.Unlock()
					chunk.Store(nChunks) // the other readers stop at their next chunk
					return
				}
				reads.Add(1)
				handoffs.Add(1)
				select {
				case queue <- job{b, off}:
				default:
					full.Add(1)
					queue <- job{b, off}
				}
			}
		}()
	}

	// Wait for the readers, then the parse workers, and total their work.
	rwg.Wait()
	close(queue)
	pwg.Wait()
	st.reads, st.handoffs, st.queueFull = reads.Load(), handoffs.Load(), full.Load()
	for w := range outs {
		st.parsePages += outs[w].pages
		st.parseBusy += busy[w]
	}
	return outs, readErr
}

// parseChunk parses the leaf-set pages of one chunk into out and keeps
// the first word of every other page in nextPg.
func parseChunk(h header, b []byte, off int64, leaf bitset, nextPg []uint32, out *scanStream) {
	ps := int(h.pageSize)
	first := uint32(off/h.pageSize) + 1
	for i := 0; i < len(b)/ps; i++ {
		pg, p := first+uint32(i), b[i*ps:(i+1)*ps]
		if leaf.has(pg) {
			scanLeaf(h, pg, p, out)
		} else {
			nextPg[pg] = binary.BigEndian.Uint32(p[0:4])
		}
	}
	out.pages += int64(len(b) / ps)
}

// scanProbe waits o.probeAt into phase 2, then projects the phase 2 time
// from the bytes scanned against the whole file and prints it. Past
// o.probeLimit it closes stop. If phase 2 ends first it records that; with
// no limit set it does nothing.
func scanProbe(o ccOpts, total int64, t2 time.Time, scanned *atomic.Int64, done <-chan struct{}, stop chan struct{}, pr *scanProbeResult) {
	// Wait for the probe time.
	if o.probeLimit <= 0 {
		return
	}
	at := o.probeAt
	if at <= 0 {
		at = time.Minute
	}
	select {
	case <-done:
		pr.line = fmt.Sprintf("probe: phase 2 ended in %s, before the %s probe, within limit %s",
			time.Since(t2).Round(time.Millisecond), at, o.probeLimit)
		return
	case <-time.After(at):
	}

	// Project and report; stop past the limit.
	el, n := time.Since(t2), scanned.Load()
	proj := time.Duration(float64(el) * float64(total) / float64(max(n, 1)))
	pr.stopped = proj > o.probeLimit
	pr.line = fmt.Sprintf("probe %s: %s into phase 2, scanned %.0f of %.0f MB, projected phase 2 time %s against limit %s (%s)",
		time.Now().UTC().Format("15:04:05Z"), el.Round(time.Second), float64(n)/1e6, float64(total)/1e6, proj.Round(time.Second), o.probeLimit,
		map[bool]string{true: "over, stopping", false: "within"}[pr.stopped])
	if o.out != nil {
		fmt.Fprintln(o.out, pr.line)
	}
	if pr.stopped {
		close(stop)
	}
}

// scanProbeResult is what the phase 2 probe decided.
type scanProbeResult struct {
	line    string
	stopped bool
}

// scanLeaf parses one leaf-set page into out without allocating: its type,
// cell count and overflow heads, with the local split crosscheck uses.
func scanLeaf(h header, pg uint32, p []byte, out *scanStream) {
	// The page must be a leaf whose cell pointers fit.
	u := int(h.usable)
	t := p[0]
	if t != 0x0d && t != 0x0a {
		out.bad = append(out.bad, badPage{page: pg, err: fmt.Sprintf("leaf-set page has b-tree type 0x%02x", t)})
		return
	}
	n := int(binary.BigEndian.Uint16(p[3:]))
	if 8+2*n > u {
		out.bad = append(out.bad, badPage{page: pg, err: fmt.Sprintf("%d cells do not fit", n)})
		return
	}

	// Parse each cell; a bad cell drops the page's heads, as crosscheck
	// drops a bad page's children.
	mark := len(out.heads)
	if err := scanCells(p, u, t, n, pg, &out.heads); err != "" {
		out.heads = out.heads[:mark]
		out.bad = append(out.bad, badPage{page: pg, err: err})
		return
	}

	// Count the page by kind and type.
	k := kindTable
	if t == 0x0a {
		k = kindIndex
	}
	out.kinds[k]++
	out.byType[t]++
	out.cells += uint64(n)
}

// scanCells appends the overflow heads of a leaf's n cells, returning a
// reason if a cell does not parse.
func scanCells(p []byte, u int, t byte, n int, pg uint32, heads *[]chainHead) string {
	for i := 0; i < n; i++ {
		// Find the cell and its payload size.
		off := int(binary.BigEndian.Uint16(p[8+2*i:]))
		if off < 8 || off+4 > u {
			return fmt.Sprintf("cell %d offset %d", i, off)
		}
		c := p[off:u]
		size, k := varint(c)
		if k == 0 {
			return fmt.Sprintf("cell %d payload size", i)
		}
		c = c[k:]

		// Skip a table leaf's rowid.
		if t == 0x0d {
			if _, k = varint(c); k == 0 {
				return fmt.Sprintf("cell %d rowid", i)
			}
			c = c[k:]
		}

		// Record the overflow head past the local payload.
		local := localPayload(u, int64(size), t == 0x0d)
		if int64(len(c)) < local+4 && int64(size) > local || int64(len(c)) < local {
			return fmt.Sprintf("cell %d payload %d runs past the page", i, size)
		}
		if int64(size) > local {
			pages := (int64(size) - local + int64(u-4) - 1) / int64(u-4)
			*heads = append(*heads, chainHead{pg: binary.BigEndian.Uint32(c[local:]), pages: uint32(pages), parent: pg})
		}
	}
	return ""
}

// scanChain follows one overflow chain through nextPg for the page count
// its payload implies, marking each page reached and checking it against
// the freelist, the b-tree and leaf sets, and its length.
func scanChain(res *ccResult, h header, hd chainHead, nextPg []uint32, reached, btree, leaf, free bitset,
	addConflict func(conflict), addBad func(badPage)) {
	prev, cur := hd.parent, hd.pg
	for i := uint32(0); i < hd.pages; i++ {
		// The page must be in range and not reached by anything else.
		if cur < 1 || cur > h.dbPages {
			addBad(badPage{page: prev, err: fmt.Sprintf("points to page %d outside 1..%d", cur, h.dbPages), ptr: true})
			return
		}
		if cur == lockBytePage(h) {
			addBad(badPage{page: prev, err: fmt.Sprintf("points to the lock-byte page %d", cur), ptr: true})
			return
		}
		if reached.has(cur) {
			if btree.has(cur) || leaf.has(cur) {
				res.chainInTree++
			}
			addConflict(conflict{page: cur, parent: prev, parentOv: i > 0, kind: kindOverflow, dup: true, parent2: prev, parent2Ov: i > 0})
			return
		}

		// Claim it; a free one conflicts.
		reached.set(cur)
		if free.has(cur) {
			addConflict(conflict{page: cur, parent: prev, parentOv: i > 0, kind: kindOverflow})
		}

		// The chain must end with next 0 exactly at its implied length.
		nx := nextPg[cur]
		switch last := i == hd.pages-1; {
		case last && nx != 0:
			addBad(badPage{page: cur, err: fmt.Sprintf("overflow chain goes on to %d past its length", nx)})
			return
		case !last && nx == 0:
			addBad(badPage{page: cur, err: fmt.Sprintf("overflow chain ends %d pages early", hd.pages-1-i)})
			return
		}
		res.kinds[kindOverflow]++
		prev, cur = cur, nx
	}
}

// scanProgress prints one progress line: phase 1 by pages read against
// the pages in use, phase 2 by bytes against the file, with its rate since
// phase 2 began, the time left and the running problem totals.
func scanProgress(out io.Writer, el time.Duration, h header, inUse uint32, phase, pages, reads, scanned, conf, dup, bad int64) {
	// Phase 1 and 3 report pages; their time left is not projected.
	now := time.Now().UTC().Format("15:04:05Z")
	sec := el.Seconds()
	if phase != 2 {
		fmt.Fprintf(out, "progress %s: scan phase %d, read %d pages (%.1f%% of %d in use) in %d reads; conflicts %d (reached twice %d), bad %d\n",
			now, phase, pages, 100*float64(pages)/float64(inUse), inUse, reads, conf, dup, bad)
		return
	}

	// Phase 2 projects the time left from the bytes scanned so far.
	total := int64(h.dbPages) * h.pageSize
	eta := "unknown"
	if rate := float64(scanned) / sec; rate > 0 {
		eta = (time.Duration(float64(total-scanned)/rate) * time.Second).Round(time.Second).String()
	}
	fmt.Fprintf(out, "progress %s: scan phase 2, scanned %.0f of %.0f MB (%.1f%%, %d pages in use), %.1f MB/s, est. %s left; conflicts %d (reached twice %d), bad %d\n",
		now, float64(scanned)/1e6, float64(total)/1e6, 100*float64(scanned)/float64(total), inUse, float64(scanned)/1e6/sec, eta, conf, dup, bad)
}

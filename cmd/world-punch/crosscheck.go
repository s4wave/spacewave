package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// pageKind is what the crosscheck reached a page as.
type pageKind uint8

const (
	kindTable    pageKind = iota // table b-tree page (0x05, 0x0d)
	kindIndex                    // index b-tree page (0x02, 0x0a)
	kindOverflow                 // overflow chain page
	kindTree                     // child pointer target, type not read yet
	numKinds
)

func (k pageKind) String() string {
	return [...]string{"table", "index", "overflow", "tree"}[k]
}

// ccItem is a page to read: reached from parent, as kind. left is the
// number of chain pages from this one to the end of an overflow chain.
type ccItem struct {
	pg, parent uint32
	kind       pageKind
	left       uint32
	schema     bool // page of the sqlite_master tree
	fromOv     bool // parent is an overflow page
}

type conflict struct {
	page, parent uint32
	parentOv     bool // parent is an overflow page
	kind         pageKind
	dup          bool // reached twice (from parent and parent2), not reached and free
	parent2      uint32
	parent2Ov    bool
	late         bool   // found by the late walk
	class        string // explained reason after recheck, "" if unexplained
}

type badPage struct {
	page  uint32
	err   string
	freed bool // on a freelist snapshot taken during or after the walk
	ptr   bool // page is a parent with an out-of-range pointer
}

type ccOpts struct {
	readers int
	mbps    float64
	retries int
	retryIn time.Duration
	every   time.Duration // progress interval, 0 for none
	// mbpsFile, if set, is reread every second; a number in it replaces
	// the cap.
	mbpsFile string
	out      io.Writer
	// bulk, if set, serves the tree walk's reads (an O_DIRECT descriptor
	// that does not fill the page cache); its buffers are 4096-aligned.
	bulk io.ReaderAt
	// snapEvery, if set, walks the freelist this often during the tree
	// walk; a page on any snapshot was freed during the walk.
	snapEvery time.Duration
	// freelistOverride lets tests edit the free set of a walk.
	freelistOverride func(free bitset, recheck bool)
	// interrupt, when closed, stops the walk: no new reads are issued,
	// the recheck and late walk are skipped, and the result is marked
	// interrupted.
	interrupt <-chan struct{}
	// streams is the number of parallel sequential streams of scanCheck,
	// parsers its parse workers.
	streams, parsers int
	// probeLimit, if set, stops scanCheck when probeAt (default a minute)
	// into phase 2 its projected phase 2 time exceeds it.
	probeLimit, probeAt time.Duration
}

type ccResult struct {
	dbPages     uint32
	reached     uint64
	free        uint64
	lockPage    uint64
	unaccounted uint64
	kinds       [numKinds]uint64
	leafCells   uint64
	firstOfKind [numKinds]uint32
	conflicts   []conflict
	bad         []badPage
	freeTorn    error
	readBytes   int64
	readPages   int64
	reads       int64  // read calls (runs of adjacent pages)
	inUse       uint32 // header pages minus header freelist count at start
	rounds      int
	serial      time.Duration // sort and enqueue between rounds, outside reads
	interrupted bool
	elapsed     time.Duration

	// Set by scanCheck only.
	phase       string           // phase an interrupt stopped
	phaseTimes  [3]time.Duration // interior walk, sequential scan, chains
	scanReads   int64            // phase 2 reads, counted in reads
	scanBytes   int64            // phase 2 bytes
	chainInTree int              // chain pages that are b-tree or leaf pages
	pass        scanPassStats
	probe       scanProbeResult
	unaccPages  []uint32
	unaccFreed  uint64 // unaccounted pages on a later freelist snapshot
	freeTrunks  uint64
	freeLeaves  uint64
	byType      map[byte]uint64
	startFree   uint32 // header freelist counts
	endFree     uint32
	endPages    uint32
	startChange uint32 // header change counters (scan)
	endChange   uint32
	strict      bool // scan: header freelist count and end header must match

	lateRoots        int
	lateReached      int
	leftoverInterior int // leftover pages that read as interior pages
	leftoverLeaf     int // leftover leaf, overflow or other pages
	leftoverUnparse  int
	leftoverByType   map[string]int
}

func (r *ccResult) unaccUnexplained() uint64 { return r.unaccounted - r.unaccFreed }

func (r *ccResult) badFreed() int {
	n := 0
	for _, b := range r.bad {
		if b.freed {
			n++
		}
	}
	return n
}

// sanityMatch reports whether reached, free and lock-byte pages sum to the
// header page count.
func (r *ccResult) sanityMatch() bool {
	return r.reached+r.free+r.lockPage == uint64(r.dbPages)
}

// failReasons lists why the run fails: an interrupted walk, a sanity
// mismatch, unexplained unaccounted pages, unexplained bad pages,
// unexplained conflicts, or a freelist walk that stopped early; for a scan
// also a header freelist count unlike the walk, or a header that changed
// between the start and the end. None means PASS.
func (r *ccResult) failReasons() []string {
	checks := []struct {
		fail bool
		why  string
	}{
		{r.interrupted, "interrupted"},
		{!r.sanityMatch(), "sanity match false"},
		{r.unaccUnexplained() > 0, fmt.Sprintf("unaccounted-unexplained %d", r.unaccUnexplained())},
		{len(r.bad) > r.badFreed(), fmt.Sprintf("bad-unexplained %d", len(r.bad)-r.badFreed())},
		{r.unexplained() > 0, fmt.Sprintf("unexplained conflicts %d", r.unexplained())},
		{r.freeTorn != nil, "freelist walk stopped early"},
		{r.strict && uint64(r.startFree) != r.freeTrunks+r.freeLeaves, fmt.Sprintf("header freelist count %d != walked trunks %d + leaves %d", r.startFree, r.freeTrunks, r.freeLeaves)},
		{r.strict && !r.interrupted && (r.endPages != r.dbPages || r.endFree != r.startFree || r.endChange != r.startChange),
			fmt.Sprintf("file changed during scan: header pages %d -> %d, freelist count %d -> %d, change counter %d -> %d", r.dbPages, r.endPages, r.startFree, r.endFree, r.startChange, r.endChange)},
	}
	var why []string
	for _, c := range checks {
		if c.fail {
			why = append(why, c.why)
		}
	}
	return why
}

// failed reports whether failReasons has any reason.
func (r *ccResult) failed() bool { return len(r.failReasons()) > 0 }

// avgRead is the mean pages per read call.
func (r *ccResult) avgRead() float64 {
	if r.reads == 0 {
		return 0
	}
	return float64(r.readPages) / float64(r.reads)
}

// dups counts reached-twice conflicts.
func (r *ccResult) dups() int {
	n := 0
	for _, c := range r.conflicts {
		if c.dup {
			n++
		}
	}
	return n
}

// printPartial reports an interrupted walk: totals so far and the first
// conflicts, classified only if the recheck ran.
func (r *ccResult) printPartial(out io.Writer) {
	// Report how far the walk got against the pages in use.
	pct := 0.0
	if r.inUse > 0 {
		pct = 100 * float64(r.readPages) / float64(r.inUse)
	}
	if r.phase != "" {
		fmt.Fprintf(out, "scan stopped in %s\n", r.phase)
	}
	fmt.Fprintf(out, "INTERRUPTED at round %d: read %d pages (%.1f%% of %d in use) in %d reads (%.2f pages per read)\n",
		r.rounds, r.readPages, pct, r.inUse, r.reads, r.avgRead())

	// List the totals and the first conflicts found so far.
	fmt.Fprintf(out, "partial totals: conflicts %d (reached twice %d), bad %d\n", len(r.conflicts), r.dups(), len(r.bad))
	for i, c := range r.conflicts {
		if i == 20 {
			fmt.Fprintf(out, "  ... %d more\n", len(r.conflicts)-i)
			break
		}
		what := "reached and free"
		if c.dup {
			what = "reached twice"
		}
		class := c.class
		if class == "" {
			class = "unclassified"
		}
		fmt.Fprintf(out, "  page %d (%s, parent %d) %s: %s\n", c.page, c.kind, c.parent, what, class)
	}

	// List the first bad pages.
	for i, b := range r.bad {
		if i == 10 {
			break
		}
		fmt.Fprintf(out, "  bad page %d: %s\n", b.page, b.err)
	}

	// Conclude nothing from a partial walk.
	fmt.Fprintf(out, "result: INCOMPLETE (interrupted, nothing concluded)\n")
	r.printRead(out)
}

// printScan prints the scan's phase times and total, the parse workers'
// rate and queue, and chain pages found in the b-tree or leaf sets.
func (r *ccResult) printScan(out io.Writer) {
	// Phase times and total.
	p := r.pass
	fmt.Fprintf(out, "scan: phase 1 (interior walk) %s, phase 2 (sequential scan) %s at %.1f MB/s in %d reads, phase 3 (chains) %s, total %s\n",
		r.phaseTimes[0].Round(time.Millisecond), r.phaseTimes[1].Round(time.Millisecond),
		float64(r.scanBytes)/1e6/max(r.phaseTimes[1].Seconds(), 1e-9), r.scanReads,
		r.phaseTimes[2].Round(time.Millisecond), r.elapsed.Round(time.Millisecond))

	// Parse rate per busy worker and whether the queue ever filled.
	rate := 0.0
	if p.parsers > 0 && p.parseBusy > 0 {
		rate = float64(p.parsePages) / p.parseBusy.Seconds()
	}
	fmt.Fprintf(out, "scan parse: %d workers, %.0f pages/s per worker while busy, busy %s of %s per worker; queue full on %d of %d handoffs\n",
		p.parsers, rate, (p.parseBusy / time.Duration(max(p.parsers, 1))).Round(time.Millisecond), r.phaseTimes[1].Round(time.Millisecond), p.queueFull, p.handoffs)
	fmt.Fprintf(out, "scan: chain pages in the tree or leaf set %d\n", r.chainInTree)
	if r.probe.line != "" {
		fmt.Fprintln(out, r.probe.line)
	}
}

// printRead prints the read totals and rates.
func (r *ccResult) printRead(out io.Writer) {
	sec := r.elapsed.Seconds()
	fmt.Fprintf(out, "read %d bytes in %s (%.1f MB/s, %.0f pages/s) in %d reads (%.2f pages per read); %d rounds, serial between rounds %s\n",
		r.readBytes, r.elapsed.Round(time.Second), float64(r.readBytes)/1e6/sec, float64(r.readPages)/sec,
		r.reads, r.avgRead(), r.rounds, r.serial.Round(time.Millisecond))
}

func (r *ccResult) unexplained() int {
	n := 0
	for _, c := range r.conflicts {
		if c.class == "" {
			n++
		}
	}
	return n
}

// print writes the full report and the PASS or FAIL result.
func (r *ccResult) print(out io.Writer) {
	// Report the sanity sum and the page counts by type.
	sum := r.reached + r.free + r.lockPage
	fmt.Fprintf(out, "sanity: reached %d + free %d + lock-byte %d = %d of header page count %d (match %v); leaf cells %d\n",
		r.reached, r.free, r.lockPage, sum, r.dbPages, sum == uint64(r.dbPages), r.leafCells)
	fmt.Fprintf(out, "header pages start %d end %d; header freelist count start %d end %d\n", r.dbPages, r.endPages, r.startFree, r.endFree)
	fmt.Fprintf(out, "pages walked %d (main walk %d + late walk %d, late roots not counted) = table interior %d + table leaf %d + index interior %d + index leaf %d + overflow %d\n",
		r.reached+uint64(r.lateReached), r.reached, r.lateReached, r.byType[0x05], r.byType[0x0d], r.byType[0x02], r.byType[0x0a], r.kinds[kindOverflow])
	fmt.Fprintf(out, "free at start %d = trunks %d + leaves %d\n", r.free, r.freeTrunks, r.freeLeaves)

	// List the first unaccounted pages.
	fmt.Fprintf(out, "unaccounted-total %d, unaccounted-freed-during-walk %d, unaccounted-unexplained %d (main walk; the late walk and leftovers follow)\n", r.unaccounted, r.unaccFreed, r.unaccUnexplained())
	var unexp []uint32
	for _, p := range r.unaccPages {
		if len(unexp) == 50 {
			break
		}
		unexp = append(unexp, p)
	}
	if len(unexp) > 0 {
		fmt.Fprintf(out, "unaccounted pages (first %d, freed or not): %v\n", len(unexp), unexp)
	}

	// List the unexplained bad pages and a torn freelist.
	fmt.Fprintf(out, "bad-total %d, bad-freed-during-walk %d, bad-unexplained %d\n", len(r.bad), r.badFreed(), len(r.bad)-r.badFreed())
	if r.freeTorn != nil {
		fmt.Fprintln(out, "freelist walk stopped early:", r.freeTorn)
	}
	shownBad := 0
	for _, b := range r.bad {
		if b.freed || shownBad == 50 {
			continue
		}
		shownBad++
		fmt.Fprintf(out, "  bad page %d (UNEXPLAINED): %s\n", b.page, b.err)
	}

	// List the unexplained conflicts and a sample of the explained, then
	// the count of each class.
	fmt.Fprintf(out, "conflicts %d\n", len(r.conflicts))
	classes := map[string]int{}
	shown := 0
	for _, c := range r.conflicts {
		classes[c.class]++
		if c.class != "" && shown >= 20 {
			continue
		}
		shown++
		what := "reached and free"
		if c.dup {
			what = "reached twice"
		}
		class := c.class
		if class == "" {
			class = "UNEXPLAINED"
		}
		if c.late {
			what += " (late walk)"
		}
		fmt.Fprintf(out, "  page %d (%s, parent %d) %s: %s\n", c.page, c.kind, c.parent, what, class)
	}
	for k, n := range classes {
		if k != "" {
			fmt.Fprintf(out, "  %d %s\n", n, k)
		}
	}

	// Report the late walk and the leftovers.
	fmt.Fprintf(out, "late walk: %d roots left unreached by any walk, %d pages walked\n", r.lateRoots, r.lateReached)
	fmt.Fprintf(out, "leftover (unaccounted or bad, not freed during the walk, after the late walk) by type: %v\n", r.leftoverByType)
	fmt.Fprintf(out, "leftover interior %d\n", r.leftoverInterior)
	fmt.Fprintf(out, "leftover leaf+overflow %d\n", r.leftoverLeaf)
	fmt.Fprintf(out, "leftover unparsable %d\n", r.leftoverUnparse)
	fmt.Fprintf(out, "freed-during-walk %d (unaccounted %d + bad %d) against freelist growth %d (end %d - start %d)\n",
		r.unaccFreed+uint64(r.badFreed()), r.unaccFreed, r.badFreed(), int64(r.endFree)-int64(r.startFree), r.endFree, r.startFree)

	// Conclude: FAIL with every reason, else PASS.
	fmt.Fprintf(out, "unexplained conflicts %d\n", r.unexplained())
	if why := r.failReasons(); len(why) > 0 {
		fmt.Fprintf(out, "result: FAIL (%s)\n", strings.Join(why, ", "))
	} else {
		fmt.Fprintf(out, "result: PASS\n")
	}
	r.printRead(out)
}

// limiter paces reads to mbps across readers.
type limiter struct {
	mu   sync.Mutex
	rate float64 // bytes per second, 0 for no cap
	next time.Time
}

func (l *limiter) set(mbps float64) {
	l.mu.Lock()
	l.rate = mbps * 1e6
	l.mu.Unlock()
}

func (l *limiter) wait(n int) {
	l.mu.Lock()
	if l.rate <= 0 {
		l.mu.Unlock()
		return
	}
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	at := l.next
	l.next = l.next.Add(time.Duration(float64(n) / l.rate * float64(time.Second)))
	l.mu.Unlock()
	time.Sleep(time.Until(at))
}

// crosscheck walks every b-tree from page 1 and the roots in sqlite_master,
// through interior and leaf pages and overflow chains, and checks that no
// reached page is on the freelist or reached twice. It only reads.
func crosscheck(f io.ReaderAt, h header, o ccOpts) (ccResult, error) {
	// Size the result against the header: pages in use exclude the
	// header's freelist count.
	start := time.Now()
	res := ccResult{dbPages: h.dbPages, byType: map[byte]uint64{}, startFree: h.freeCount}
	if h.freeCount < h.dbPages {
		res.inUse = h.dbPages - h.freeCount
	}
	if res.inUse == 0 {
		res.inUse = h.dbPages
	}
	if o.readers <= 0 {
		o.readers = 8
	}

	// Walk the freelist once for the start free set.
	free, fw := freeSet(f, h)
	res.freeTorn = fw.torn
	res.freeTrunks, res.freeLeaves = uint64(len(fw.trunks)), uint64(len(fw.leaves))
	if o.freelistOverride != nil {
		o.freelistOverride(free, false)
	}

	// everFree collects every freelist snapshot taken during or after the walk.
	everFree := newBitset(h.dbPages)
	var everMu sync.Mutex
	addFree := func(b bitset) {
		everMu.Lock()
		for i := range everFree {
			everFree[i] |= b[i]
		}
		everMu.Unlock()
	}

	// Track each page the walk reaches, its parent and the conflicting kinds.
	reached := newBitset(h.dbPages)
	kindOf := map[uint32]pageKind{} // kinds of conflicting pages only
	parents := make([]uint32, uint64(h.dbPages)+1)
	fromOv := newBitset(h.dbPages)
	lim := &limiter{rate: o.mbps * 1e6}

	// Snapshot the freelist and watch the rate file until the walk returns.
	stop := make(chan struct{})
	defer close(stop)
	if o.snapEvery > 0 {
		go func() {
			tk := time.NewTicker(o.snapEvery)
			defer tk.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tk.C:
					b, _ := freeSet(f, h)
					addFree(b)
				}
			}
		}()
	}
	if o.mbpsFile != "" {
		go watchMbps(o.mbpsFile, o.mbps, lim, o.out, stop)
	}

	// Count reads under mu and found problems in atomics, for the
	// progress line; the walk goroutine alone appends to res.
	var mu sync.Mutex
	var readBytes, readPages, reads int64
	round := 0
	var nConf, nDup, nBad atomic.Int64
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
	interrupted := func() bool {
		select {
		case <-o.interrupt:
			return true
		default:
			return false
		}
	}

	// Print progress every o.every until the walk returns.
	if o.every > 0 && o.out != nil {
		go func() {
			tk := time.NewTicker(o.every)
			defer tk.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tk.C:
					mu.Lock()
					b, p, n, r := readBytes, readPages, reads, round
					mu.Unlock()
					printProgress(o.out, time.Since(start), res.inUse, r, b, p, n, nConf.Load(), nDup.Load(), nBad.Load())
				}
			}
		}()
	}

	// lateRoot holds late-walk roots: marked reached so the walk does not
	// re-read them, but claimed without a dup when a walked page points
	// to one, since then it was not a root at all.
	cur, late := free, false
	lateRoot := newBitset(h.dbPages)
	enqueue := func(next []ccItem, it ccItem) []ccItem {
		// Drop pointers out of range and late chains run into reached pages.
		if it.pg < 1 || it.pg > h.dbPages {
			addBad(badPage{page: it.parent, err: fmt.Sprintf("points to page %d outside 1..%d", it.pg, h.dbPages), ptr: true})
			return next
		}
		if it.pg == lockBytePage(h) {
			addBad(badPage{page: it.parent, err: fmt.Sprintf("points to the lock-byte page %d", it.pg), ptr: true})
			return next
		}
		if it.kind == kindOverflow && it.left == 0 && reached.has(it.pg) {
			return next // a late chain of unknown length ran into reached pages
		}

		// Claim a late root a walked page points to.
		if lateRoot.has(it.pg) {
			lateRoot.clear(it.pg)
			res.lateReached++
			parents[it.pg] = it.parent
			if it.fromOv {
				fromOv.set(it.pg)
			}
			if cur.has(it.pg) {
				addConflict(conflict{page: it.pg, parent: it.parent, parentOv: it.fromOv, kind: it.kind, late: late})
				kindOf[it.pg] = it.kind
			}
			return next // its children were enqueued as a root's
		}

		// A page reached twice conflicts.
		if reached.has(it.pg) {
			addConflict(conflict{page: it.pg, parent: parents[it.pg], parentOv: fromOv.has(it.pg),
				kind: it.kind, dup: true, parent2: it.parent, parent2Ov: it.fromOv, late: late})
			return next
		}

		// Mark the page reached; a free one conflicts.
		reached.set(it.pg)
		parents[it.pg] = it.parent
		if it.fromOv {
			fromOv.set(it.pg)
		}
		if late {
			res.lateReached++
		}
		if cur.has(it.pg) {
			addConflict(conflict{page: it.pg, parent: it.parent, parentOv: it.fromOv, kind: it.kind, late: late})
			kindOf[it.pg] = it.kind
		}
		return append(next, it)
	}

	// runWalk reads the queue round by round, timing the serial sort and
	// enqueue outside the reads, until it empties or is interrupted. A
	// failed or short read ends the walk with that error.
	runWalk := func(queue []ccItem) error {
		for len(queue) > 0 && !interrupted() {
			// Sort the round by page number so adjacent pages merge into runs.
			t0 := time.Now()
			mu.Lock()
			round++
			mu.Unlock()
			slices.SortFunc(queue, func(a, b ccItem) int { return int(a.pg) - int(b.pg) })
			res.serial += time.Since(t0)

			// Read the round, counting each read call and its pages.
			bulk := o.bulk
			if bulk == nil {
				bulk = f
			}
			results, err := readRound(bulk, h, queue, o.readers, lim, o.interrupt, func(n int) {
				mu.Lock()
				readBytes, readPages, reads = readBytes+int64(n)*h.pageSize, readPages+int64(n), reads+1
				mu.Unlock()
			})
			if err != nil {
				return err
			}

			// Tally each page and enqueue its children for the next round.
			t1 := time.Now()
			var next []ccItem
			for _, r := range results {
				next = tally(&res, kindOf, r, addBad, next, enqueue)
			}
			queue = next
			res.serial += time.Since(t1)
		}
		return nil
	}

	// Walk from page 1 and give conflicts the kind their page read as.
	if err := runWalk(enqueue(nil, ccItem{pg: 1, kind: kindTree, schema: true})); err != nil {
		return res, err
	}
	for i := range res.conflicts {
		if k, ok := kindOf[res.conflicts[i].page]; ok {
			res.conflicts[i].kind = k
		}
	}

	// Stop with the totals so far if interrupted.
	finish := func() {
		mu.Lock()
		res.readBytes, res.readPages, res.reads, res.rounds = readBytes, readPages, reads, round
		mu.Unlock()
		res.elapsed = time.Since(start)
	}
	if interrupted() {
		res.interrupted = true
		finish()
		return res, nil
	}
	res.readBytes = readBytes

	// Count pages reached, free and neither; a reached lock-byte page is
	// bad.
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

	// Recheck conflicts against fresh freelists, stopping if interrupted.
	final := recheck(f, h, o, &res, addFree)
	if interrupted() {
		res.interrupted = true
		finish()
		return res, nil
	}

	// Late walk: an unaccounted or bad page not on the final freelist is
	// read again and, if it parses, walked as a root against the final
	// free set, so a subtree the main walk lost is still checked.
	cur, late = final, true
	tried := newBitset(h.dbPages)
	type lateRead struct {
		pg   uint32
		kind string
		r    pageResult
	}
	var leftovers []lateRead
	page := make([]byte, h.pageSize)
	for lr := 0; lr < 3; lr++ {
		var cands []uint32
		for _, pg := range res.unaccPages {
			if !reached.has(pg) && !final.has(pg) && !tried.has(pg) {
				cands = append(cands, pg)
			}
		}
		for _, b := range res.bad {
			if !b.ptr && b.page <= h.dbPages && !final.has(b.page) && !tried.has(b.page) {
				cands = append(cands, b.page)
			}
		}
		if len(cands) == 0 {
			break
		}
		reads := make([]lateRead, 0, len(cands))
		childOf := newBitset(h.dbPages)
		for _, pg := range cands {
			tried.set(pg)
			lr := lateRead{pg: pg, kind: "unparsable"}
			if _, err := f.ReadAt(page, int64(pg-1)*h.pageSize); err == nil {
				lr.r = parsePage(h, ccItem{pg: pg, kind: kindTree}, page)
				if lr.r.bad == "" {
					lr.kind = map[byte]string{0x05: "table interior", 0x0d: "table leaf", 0x02: "index interior", 0x0a: "index leaf"}[lr.r.ptype]
				} else if next := binary.BigEndian.Uint32(page[0:4]); next <= h.dbPages {
					lr.kind = "overflow/other"
					lr.r = pageResult{item: ccItem{pg: pg, kind: kindOverflow}, tree: kindOverflow}
					if next != 0 {
						lr.r.children = []ccItem{{pg: next, parent: pg, kind: kindOverflow, fromOv: true}}
					}
				} else {
					lr.r = pageResult{}
				}
			}
			for _, c := range lr.r.children {
				if c.pg >= 1 && c.pg <= h.dbPages {
					childOf.set(c.pg)
				}
			}
			reads = append(reads, lr)
		}
		var next []ccItem
		var roots []lateRead
		for _, lr := range reads {
			if childOf.has(lr.pg) {
				continue // reached from another late root
			}
			reached.set(lr.pg)
			lateRoot.set(lr.pg)
			roots = append(roots, lr)
			for _, c := range lr.r.children {
				next = enqueue(next, c)
			}
		}
		if err := runWalk(next); err != nil {
			return res, err
		}
		for _, lr := range roots {
			if lateRoot.has(lr.pg) {
				lateRoot.clear(lr.pg)
				res.lateRoots++
				leftovers = append(leftovers, lr)
				continue
			}
			// Claimed by the walk: count it as a walked page.
			if lr.kind == "unparsable" {
				continue
			} else if lr.r.tree == kindOverflow {
				res.kinds[kindOverflow]++
			} else {
				res.kinds[lr.r.tree]++
				res.byType[lr.r.ptype]++
				res.leafCells += uint64(lr.r.leafCells)
			}
		}
		// Candidates no root reached (a cycle among them) stay leftover.
		for _, lr := range reads {
			if !reached.has(lr.pg) {
				reached.set(lr.pg)
				leftovers = append(leftovers, lr)
			}
		}
	}
	for i := range res.conflicts {
		if k, ok := kindOf[res.conflicts[i].page]; ok {
			res.conflicts[i].kind = k
		}
	}
	if res.lateReached > 0 {
		recheck(f, h, o, &res, addFree)
	}

	// A page neither reached nor free at the start, or that read as a bad
	// page, is explained if a freelist snapshot during or after the walk
	// has it: a live write freed it while the walk ran.
	everMu.Lock()
	for _, pg := range res.unaccPages {
		if everFree.has(pg) {
			res.unaccFreed++
		}
	}
	for i := range res.bad {
		if pg := res.bad[i].page; pg <= h.dbPages && everFree.has(pg) {
			res.bad[i].freed = true
		}
	}

	// Count the leftovers no snapshot explains, by the type they read as.
	res.leftoverByType = map[string]int{}
	for _, lr := range leftovers {
		if everFree.has(lr.pg) {
			continue
		}
		res.leftoverByType[lr.kind]++
		switch lr.kind {
		case "table interior", "index interior":
			res.leftoverInterior++
		case "unparsable":
			res.leftoverUnparse++
		default:
			res.leftoverLeaf++
		}
	}
	everMu.Unlock()

	// Read the header again for the end page and freelist counts.
	var hb [100]byte
	if _, err := f.ReadAt(hb[:], 0); err == nil {
		res.endPages = binary.BigEndian.Uint32(hb[28:32])
		res.endFree = binary.BigEndian.Uint32(hb[36:40])
	}

	// Record the read totals; an interrupt during the late walk still
	// marks the run incomplete.
	finish()
	res.interrupted = interrupted()
	return res, nil
}

// tally counts one page read by the walk, records a bad page, and enqueues
// its children onto next.
func tally(res *ccResult, kindOf map[uint32]pageKind, r pageResult, addBad func(badPage), next []ccItem, enqueue func([]ccItem, ccItem) []ccItem) []ccItem {
	// A page that does not parse is bad and has no children.
	if r.bad != "" {
		addBad(badPage{page: r.item.pg, err: r.bad})
		return next
	}

	// Count the page by the kind it read as.
	k := r.item.kind
	if k == kindTree {
		k = r.tree
	}
	res.kinds[k]++
	if k != kindOverflow {
		res.byType[r.ptype]++
	}
	res.leafCells += uint64(r.leafCells)
	if res.firstOfKind[k] == 0 && r.item.pg != 1 {
		res.firstOfKind[k] = r.item.pg
	}

	// Resolve a conflict's kind and enqueue the children.
	if c, ok := kindOf[r.item.pg]; ok && c == kindTree {
		kindOf[r.item.pg] = k
	}
	for _, c := range r.children {
		next = enqueue(next, c)
	}
	return next
}

// printProgress prints one progress line: pages read against the pages in
// use, rates, pages per read, an estimate of the time left and the running
// problem totals.
func printProgress(out io.Writer, el time.Duration, inUse uint32, round int, bytes, pages, reads, conf, dup, bad int64) {
	// Derive the mean read size and the time left at the rate so far.
	sec := el.Seconds()
	avg, eta := 0.0, "unknown"
	if reads > 0 {
		avg = float64(pages) / float64(reads)
	}
	if rate := float64(pages) / sec; rate > 0 && int64(inUse) > pages {
		eta = (time.Duration(float64(int64(inUse)-pages)/rate) * time.Second).Round(time.Second).String()
	}

	// Keep the "progress T: round R, read P pages (X% of N in use)" prefix.
	fmt.Fprintf(out, "progress %s: round %d, read %d pages (%.1f%% of %d in use), %.0f MB at %.1f MB/s, %.0f pages/s, %d reads (%.2f pages per read), est. %s left; conflicts %d (reached twice %d), bad %d\n",
		time.Now().UTC().Format("15:04:05Z"), round, pages, 100*float64(pages)/float64(inUse), inUse, float64(bytes)/1e6, float64(bytes)/1e6/sec,
		float64(pages)/sec, reads, avg, eta, conf, dup, bad)
}

// watchMbps rereads path every second. A number in it sets the cap in
// MB/s; a missing or unreadable file restores the -mbps cap.
func watchMbps(path string, base float64, lim *limiter, out io.Writer, stop <-chan struct{}) {
	cur := base
	tk := time.NewTicker(time.Second)
	defer tk.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tk.C:
		}
		want := base
		if b, err := os.ReadFile(path); err == nil {
			if v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil && v > 0 {
				want = v
			}
		}
		if want != cur {
			cur = want
			lim.set(want)
			if out != nil {
				fmt.Fprintf(out, "rate %s: cap now %.0f MB/s (from %s)\n", time.Now().UTC().Format("15:04:05Z"), want, path)
			}
		}
	}
}

// freeSet walks the freelist tolerantly into a bitset of trunks and leaves.
func freeSet(f io.ReaderAt, h header) (bitset, *walk) {
	w, _ := walkFreelist(f, h, true)
	free := newBitset(h.dbPages)
	for _, p := range w.trunks {
		free.set(p)
	}
	for _, p := range w.leaves {
		free.set(p)
	}
	return free, w
}

// recheck rereads the freelist and each conflict's parents, up to
// o.retries times. A conflict is explained when its page has left the
// freelist, or a parent is now free or no longer points to the page: a live
// write changed it after the walk read it.
func recheck(f io.ReaderAt, h header, o ccOpts, res *ccResult, addFree func(bitset)) bitset {
	// points reports whether parent (0 for a root from sqlite_master,
	// which is not reread) still points to target.
	page := make([]byte, h.pageSize)
	points := func(free bitset, parent uint32, ov bool, target uint32) bool {
		if parent == 0 {
			return true
		}
		if free.has(parent) {
			return false
		}
		if _, err := f.ReadAt(page, int64(parent-1)*h.pageSize); err != nil {
			return true
		}
		return pointsTo(h, parent, ov, page, target)
	}

	// Retry until every conflict is explained or the tries run out.
	var last bitset
	for try := 0; try < max(o.retries, 1); try++ {
		// Wait between tries; an interrupt ends the recheck.
		if try > 0 {
			select {
			case <-time.After(o.retryIn):
			case <-o.interrupt:
				return last
			}
		}

		// Take a fresh freelist snapshot.
		open := 0
		free, _ := freeSet(f, h)
		if o.freelistOverride != nil {
			o.freelistOverride(free, true)
		}
		addFree(free)

		// Classify each open conflict against it.
		for i := range res.conflicts {
			c := &res.conflicts[i]
			switch {
			case c.class != "":
			case c.dup && (!points(free, c.parent, c.parentOv, c.page) || !points(free, c.parent2, c.parent2Ov, c.page)):
				c.class = "explained: a parent changed since the walk read it"
			case c.dup:
			case !free.has(c.page):
				c.class = "explained: no longer on the freelist"
			case !points(free, c.parent, c.parentOv, c.page):
				c.class = "explained: parent changed since the walk read it"
			}
			if c.class == "" {
				open++
			}
		}

		// Report the try and stop once nothing is open.
		if o.out != nil && len(res.conflicts) > 0 {
			fmt.Fprintf(o.out, "classify %s: try %d of %d, %d of %d conflicts still unexplained\n",
				time.Now().UTC().Format("15:04:05Z"), try+1, max(o.retries, 1), open, len(res.conflicts))
		}
		last = free
		if open == 0 {
			return last
		}
	}
	return last
}

// pointsTo reports whether page p (number pg) points to target: by its
// first word if it is an overflow page, else by a child or first overflow
// pointer of its b-tree cells.
func pointsTo(h header, pg uint32, overflow bool, p []byte, target uint32) bool {
	if overflow {
		return binary.BigEndian.Uint32(p[0:4]) == target
	}
	r := parsePage(h, ccItem{pg: pg, kind: kindTree}, p)
	for _, c := range r.children {
		if c.pg == target {
			return true
		}
	}
	return false
}

type pageResult struct {
	item      ccItem
	tree      pageKind
	ptype     byte
	leafCells int
	children  []ccItem
	bad       string
}

// readRound reads the sorted queue with readers workers, merging runs of
// adjacent pages into reads of up to 32 pages, and parses each page. A
// read that fails or returns fewer bytes than asked is a hard error: the
// round stops and nothing from that read is parsed.
func readRound(f io.ReaderAt, h header, queue []ccItem, readers int, lim *limiter, interrupt <-chan struct{}, done func(int)) ([]pageResult, error) {
	// Start readers workers, one read in flight each, each with its own
	// aligned buffer and result list; the first read error stops them.
	type run struct{ lo, hi int } // queue[lo:hi]
	runs := make(chan run, readers*2)
	results := make([][]pageResult, readers)
	var errOnce sync.Once
	var readErr error
	failed := make(chan struct{})
	var wg sync.WaitGroup
	for w := 0; w < readers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			buf := alignedBuf(int(32 * h.pageSize))
			for r := range runs {
				first := queue[r.lo].pg
				n := int(queue[r.hi-1].pg-first) + 1
				b := buf[:int64(n)*h.pageSize]
				lim.wait(len(b))
				got, err := f.ReadAt(b, int64(first-1)*h.pageSize)
				done(n)
				if err := fullRead(got, len(b), int64(first-1)*h.pageSize, err); err != nil {
					errOnce.Do(func() { readErr = err; close(failed) })
					continue
				}
				for _, it := range queue[r.lo:r.hi] {
					off := int64(it.pg-first) * h.pageSize
					results[w] = append(results[w], parsePage(h, it, b[off:off+h.pageSize]))
				}
			}
		}(w)
	}

	// Feed runs of adjacent pages until the queue ends or an interrupt.
	for lo := 0; lo < len(queue); {
		hi := lo + 1
		for hi < len(queue) && queue[hi].pg == queue[hi-1].pg+1 && queue[hi].pg-queue[lo].pg < 32 {
			hi++
		}
		select {
		case runs <- run{lo, hi}:
		case <-interrupt:
			lo = len(queue)
			continue
		case <-failed:
			lo = len(queue)
			continue
		}
		lo = hi
	}
	close(runs)
	wg.Wait()

	// Merge the workers' results.
	var all []pageResult
	for _, r := range results {
		all = append(all, r...)
	}
	return all, readErr
}

// fullRead turns a failed or short read of want bytes at off into an
// error; io.EOF with a full buffer is a full read.
func fullRead(got, want int, off int64, err error) error {
	if got == want {
		return nil
	}
	if err == nil || err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return fmt.Errorf("short read at offset %d: %d of %d bytes: %w", off, got, want, err)
}

func alignedBuf(n int) []byte {
	b := make([]byte, n+4096)
	off := 0
	if r := int(uintptrOf(b) % 4096); r != 0 {
		off = 4096 - r
	}
	return b[off : off+n]
}

// parsePage reads one page as its item's kind and returns the pages it
// points to.
func parsePage(h header, it ccItem, p []byte) pageResult {
	r := pageResult{item: it}
	if it.kind == kindOverflow {
		r.tree = kindOverflow
		next := binary.BigEndian.Uint32(p[0:4])
		switch {
		case it.left == 0: // late chain of unknown length
			if next != 0 {
				r.children = append(r.children, ccItem{pg: next, parent: it.pg, kind: kindOverflow, fromOv: true})
			}
		case it.left > 1 && next == 0:
			r.bad = fmt.Sprintf("overflow chain ends %d pages early", it.left-1)
		case it.left <= 1 && next != 0:
			r.bad = fmt.Sprintf("overflow chain goes on to %d past its length", next)
		case it.left > 1:
			r.children = append(r.children, ccItem{pg: next, parent: it.pg, kind: kindOverflow, left: it.left - 1, fromOv: true})
		}
		return r
	}
	hoff := 0
	if it.pg == 1 {
		hoff = 100
	}
	u := int(h.usable)
	t := p[hoff]
	var hlen int
	switch t {
	case 0x0d, 0x0a:
		hlen = 8
	case 0x05, 0x02:
		hlen = 12
	default:
		r.bad = fmt.Sprintf("b-tree page type 0x%02x", t)
		return r
	}
	r.tree = kindTable
	r.ptype = t
	if t == 0x02 || t == 0x0a {
		r.tree = kindIndex
	}
	child := func(pg uint32) {
		r.children = append(r.children, ccItem{pg: pg, parent: it.pg, kind: kindTree, schema: it.schema})
	}
	n := int(binary.BigEndian.Uint16(p[hoff+3:]))
	if hoff+hlen+2*n > u {
		r.bad = fmt.Sprintf("%d cells do not fit", n)
		return r
	}
	for i := 0; i < n; i++ {
		off := int(binary.BigEndian.Uint16(p[hoff+hlen+2*i:]))
		if off < hoff+hlen || off+4 > u {
			r.bad = fmt.Sprintf("cell %d offset %d", i, off)
			return r
		}
		c := p[off:u]
		if t == 0x05 || t == 0x02 {
			child(binary.BigEndian.Uint32(c))
			c = c[4:]
		}
		if t == 0x05 {
			continue
		}
		r.leafCells += boolInt(t == 0x0d || t == 0x0a)
		size, k := varint(c)
		if k == 0 {
			r.bad = fmt.Sprintf("cell %d payload size", i)
			return r
		}
		c = c[k:]
		if t == 0x0d {
			_, k = varint(c)
			if k == 0 {
				r.bad = fmt.Sprintf("cell %d rowid", i)
				return r
			}
			c = c[k:]
		}
		local := localPayload(u, int64(size), t == 0x0d)
		if int64(len(c)) < local+4 && int64(size) > local || int64(len(c)) < local {
			r.bad = fmt.Sprintf("cell %d payload %d runs past the page", i, size)
			return r
		}
		if it.schema && t == 0x0d {
			if root, ok := schemaRoot(c[:local]); ok && root != 0 {
				r.children = append(r.children, ccItem{pg: root, parent: 0, kind: kindTree})
			}
		}
		if int64(size) > local {
			pages := (int64(size) - local + int64(u-4) - 1) / int64(u-4)
			r.children = append(r.children, ccItem{pg: binary.BigEndian.Uint32(c[local:]), parent: it.pg, kind: kindOverflow, left: uint32(pages)})
		}
	}
	if hlen == 12 {
		child(binary.BigEndian.Uint32(p[hoff+8:]))
	}
	return r
}

// localPayload is how many payload bytes of a cell sit on its b-tree page.
func localPayload(u int, p int64, table bool) int64 {
	x := int64(u - 35)
	if !table {
		x = int64((u-12)*64/255 - 23)
	}
	if p <= x {
		return p
	}
	m := int64((u-12)*32/255 - 23)
	k := m + (p-m)%int64(u-4)
	if k <= x {
		return k
	}
	return m
}

// schemaRoot reads the rootpage column of a local sqlite_master record.
func schemaRoot(rec []byte) (uint32, bool) {
	hsize, k := varint(rec)
	if k == 0 || int(hsize) > len(rec) {
		return 0, false
	}
	var types []uint64
	for pos := k; pos < int(hsize); {
		st, n := varint(rec[pos:])
		if n == 0 {
			return 0, false
		}
		types = append(types, st)
		pos += n
	}
	if len(types) < 4 {
		return 0, false
	}
	body := int(hsize)
	for _, st := range types[:3] {
		body += serialLen(st)
	}
	l := serialLen(types[3])
	if body+l > len(rec) {
		return 0, false
	}
	var v uint64
	switch types[3] {
	case 8:
		return 0, true
	case 9:
		return 1, true
	case 1, 2, 3, 4:
		for _, b := range rec[body : body+l] {
			v = v<<8 | uint64(b)
		}
		return uint32(v), true
	}
	return 0, false
}

func serialLen(st uint64) int {
	switch {
	case st <= 4:
		return int(st)
	case st == 5:
		return 6
	case st == 6 || st == 7:
		return 8
	case st < 12:
		return 0
	case st%2 == 0:
		return int(st-12) / 2
	default:
		return int(st-13) / 2
	}
}

// varint decodes an SQLite varint, returning 0 bytes if it is cut off.
func varint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < 9 && i < len(b); i++ {
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

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func uintptrOf(b []byte) uintptr { return uintptr(unsafe.Pointer(&b[0])) }

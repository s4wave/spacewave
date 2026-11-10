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
	elapsed     time.Duration
	unaccPages  []uint32
	unaccFreed  uint64 // unaccounted pages on a later freelist snapshot
	freeTrunks  uint64
	freeLeaves  uint64
	byType      map[byte]uint64
	startFree   uint32 // header freelist counts
	endFree     uint32
	endPages    uint32

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

// failed reports an unexplained conflict. Leftover pages are reported
// for the operator's thresholds but do not fail the run.
func (r *ccResult) failed() bool { return r.unexplained() > 0 }

func (r *ccResult) unexplained() int {
	n := 0
	for _, c := range r.conflicts {
		if c.class == "" {
			n++
		}
	}
	return n
}

func (r *ccResult) print(out io.Writer) {
	sum := r.reached + r.free + r.lockPage
	fmt.Fprintf(out, "sanity: reached %d + free %d + lock-byte %d = %d of header page count %d (match %v); leaf cells %d\n",
		r.reached, r.free, r.lockPage, sum, r.dbPages, sum == uint64(r.dbPages), r.leafCells)
	fmt.Fprintf(out, "header pages start %d end %d; header freelist count start %d end %d\n", r.dbPages, r.endPages, r.startFree, r.endFree)
	fmt.Fprintf(out, "pages walked %d (main walk %d + late walk %d, late roots not counted) = table interior %d + table leaf %d + index interior %d + index leaf %d + overflow %d\n",
		r.reached+uint64(r.lateReached), r.reached, r.lateReached, r.byType[0x05], r.byType[0x0d], r.byType[0x02], r.byType[0x0a], r.kinds[kindOverflow])
	fmt.Fprintf(out, "free at start %d = trunks %d + leaves %d\n", r.free, r.freeTrunks, r.freeLeaves)
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
	fmt.Fprintf(out, "late walk: %d roots left unreached by any walk, %d pages walked\n", r.lateRoots, r.lateReached)
	fmt.Fprintf(out, "leftover (unaccounted or bad, not freed during the walk, after the late walk) by type: %v\n", r.leftoverByType)
	fmt.Fprintf(out, "leftover interior %d\n", r.leftoverInterior)
	fmt.Fprintf(out, "leftover leaf+overflow %d\n", r.leftoverLeaf)
	fmt.Fprintf(out, "leftover unparsable %d\n", r.leftoverUnparse)
	fmt.Fprintf(out, "freed-during-walk %d (unaccounted %d + bad %d) against freelist growth %d (end %d - start %d)\n",
		r.unaccFreed+uint64(r.badFreed()), r.unaccFreed, r.badFreed(), int64(r.endFree)-int64(r.startFree), r.endFree, r.startFree)
	fmt.Fprintf(out, "unexplained conflicts %d\n", r.unexplained())
	fmt.Fprintf(out, "result: %s\n", map[bool]string{true: "FAIL (unexplained conflicts)", false: "PASS (leftovers, if any, do not fail)"}[r.failed()])
	fmt.Fprintf(out, "read %d bytes in %s (%.1f MB/s)\n", r.readBytes, r.elapsed.Round(time.Second), float64(r.readBytes)/1e6/r.elapsed.Seconds())
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
	start := time.Now()
	res := ccResult{dbPages: h.dbPages}
	if o.readers <= 0 {
		o.readers = 8
	}
	res.byType = map[byte]uint64{}
	res.startFree = h.freeCount
	free, fw := freeSet(f, h)
	res.freeTorn = fw.torn
	res.freeTrunks, res.freeLeaves = uint64(len(fw.trunks)), uint64(len(fw.leaves))
	everFree := newBitset(h.dbPages)
	var everMu sync.Mutex
	addFree := func(b bitset) {
		everMu.Lock()
		for i := range everFree {
			everFree[i] |= b[i]
		}
		everMu.Unlock()
	}
	if o.freelistOverride != nil {
		o.freelistOverride(free, false)
	}
	reached := newBitset(h.dbPages)
	kindOf := map[uint32]pageKind{} // kinds of conflicting pages only
	parents := make([]uint32, uint64(h.dbPages)+1)
	fromOv := newBitset(h.dbPages)
	lim := &limiter{rate: o.mbps * 1e6}

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
	var mu sync.Mutex
	var readBytes, readPages int64
	round := 0
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
					b, p, r := readBytes, readPages, round
					mu.Unlock()
					el := time.Since(start).Seconds()
					fmt.Fprintf(o.out, "progress %s: round %d, read %d pages (%.1f%% of %d), %.0f MB at %.1f MB/s\n",
						time.Now().UTC().Format("15:04:05Z"), r, p, 100*float64(p)/float64(h.dbPages), h.dbPages, float64(b)/1e6, float64(b)/1e6/el)
				}
			}
		}()
	}

	cur, late := free, false
	// lateRoot holds late-walk roots: marked reached so the walk does not
	// re-read them, but claimed without a dup when a walked page points
	// to one, since then it was not a root at all.
	lateRoot := newBitset(h.dbPages)
	enqueue := func(next []ccItem, it ccItem) []ccItem {
		if it.pg < 1 || it.pg > h.dbPages {
			res.bad = append(res.bad, badPage{page: it.parent, err: fmt.Sprintf("points to page %d outside 1..%d", it.pg, h.dbPages), ptr: true})
			return next
		}
		if it.kind == kindOverflow && it.left == 0 && reached.has(it.pg) {
			return next // a late chain of unknown length ran into reached pages
		}
		if lateRoot.has(it.pg) {
			lateRoot.clear(it.pg)
			res.lateReached++
			parents[it.pg] = it.parent
			if it.fromOv {
				fromOv.set(it.pg)
			}
			if cur.has(it.pg) {
				res.conflicts = append(res.conflicts, conflict{page: it.pg, parent: it.parent, parentOv: it.fromOv, kind: it.kind, late: late})
				kindOf[it.pg] = it.kind
			}
			return next // its children were enqueued as a root's
		}
		if reached.has(it.pg) {
			res.conflicts = append(res.conflicts, conflict{page: it.pg, parent: parents[it.pg], parentOv: fromOv.has(it.pg),
				kind: it.kind, dup: true, parent2: it.parent, parent2Ov: it.fromOv, late: late})
			return next
		}
		reached.set(it.pg)
		parents[it.pg] = it.parent
		if it.fromOv {
			fromOv.set(it.pg)
		}
		if late {
			res.lateReached++
		}
		if cur.has(it.pg) {
			res.conflicts = append(res.conflicts, conflict{page: it.pg, parent: it.parent, parentOv: it.fromOv, kind: it.kind, late: late})
			kindOf[it.pg] = it.kind
		}
		return append(next, it)
	}
	runWalk := func(queue []ccItem) {
		for len(queue) > 0 {
			round++
			slices.SortFunc(queue, func(a, b ccItem) int { return int(a.pg) - int(b.pg) })
			bulk := o.bulk
			if bulk == nil {
				bulk = f
			}
			results := readRound(bulk, h, queue, o.readers, lim, func(n int) {
				mu.Lock()
				readBytes += int64(n) * h.pageSize
				readPages += int64(n)
				mu.Unlock()
			})
			var next []ccItem
			for _, r := range results {
				if r.bad != "" {
					res.bad = append(res.bad, badPage{page: r.item.pg, err: r.bad})
					continue
				}
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
				if c, ok := kindOf[r.item.pg]; ok && c == kindTree {
					kindOf[r.item.pg] = k
				}
				for _, c := range r.children {
					next = enqueue(next, c)
				}
			}
			queue = next
		}
	}
	runWalk(enqueue(nil, ccItem{pg: 1, kind: kindTree, schema: true}))
	for i := range res.conflicts {
		if k, ok := kindOf[res.conflicts[i].page]; ok {
			res.conflicts[i].kind = k
		}
	}
	res.readBytes = readBytes

	// Count pages reached, free and neither.
	lockPg := uint32(pendingByte/h.pageSize + 1)
	for pg := uint32(1); pg <= h.dbPages; pg++ {
		r, fr := reached.has(pg), free.has(pg)
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

	final := recheck(f, h, o, &res, addFree)

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
		runWalk(next)
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
	var hb [100]byte
	if _, err := f.ReadAt(hb[:], 0); err == nil {
		res.endPages = binary.BigEndian.Uint32(hb[28:32])
		res.endFree = binary.BigEndian.Uint32(hb[36:40])
	}
	res.elapsed = time.Since(start)
	return res, nil
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
	page := make([]byte, h.pageSize)
	// points reports whether parent (0 for a root from sqlite_master,
	// which is not reread) still points to target.
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
	var last bitset
	for try := 0; try < max(o.retries, 1); try++ {
		if try > 0 {
			time.Sleep(o.retryIn)
		}
		open := 0
		free, _ := freeSet(f, h)
		if o.freelistOverride != nil {
			o.freelistOverride(free, true)
		}
		addFree(free)
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
// adjacent pages into reads of up to 32 pages, and parses each page.
func readRound(f io.ReaderAt, h header, queue []ccItem, readers int, lim *limiter, done func(int)) []pageResult {
	type run struct{ lo, hi int } // queue[lo:hi]
	runs := make(chan run, readers*2)
	results := make([][]pageResult, readers)
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
				_, err := f.ReadAt(b, int64(first-1)*h.pageSize)
				done(n)
				for _, it := range queue[r.lo:r.hi] {
					if err != nil && err != io.EOF {
						results[w] = append(results[w], pageResult{item: it, bad: "read: " + err.Error()})
						continue
					}
					off := int64(it.pg-first) * h.pageSize
					results[w] = append(results[w], parsePage(h, it, b[off:off+h.pageSize]))
				}
			}
		}(w)
	}
	for lo := 0; lo < len(queue); {
		hi := lo + 1
		for hi < len(queue) && queue[hi].pg == queue[hi-1].pg+1 && queue[hi].pg-queue[lo].pg < 32 {
			hi++
		}
		runs <- run{lo, hi}
		lo = hi
	}
	close(runs)
	wg.Wait()
	var all []pageResult
	for _, r := range results {
		all = append(all, r...)
	}
	return all
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

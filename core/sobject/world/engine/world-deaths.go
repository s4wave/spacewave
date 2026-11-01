package sobject_world_engine

import (
	"slices"
	"sync"

	"github.com/s4wave/spacewave/db/block"
)

// maxWorldTransitions bounds the transitions worldDeaths holds between two
// compositions. An engine that never names a World, such as one installing
// another participant's checkpoints, reaches it and forfeits instead of
// growing.
const maxWorldTransitions = 4096

// maxWorldPuts bounds the World page puts worldDeaths holds between two
// compositions. Past it the puts are dropped and the interval forfeits, since
// a record netted without them could under-count.
const maxWorldPuts = 1 << 16

// worldTransition is one World commit: the roots it moved between and what
// its writes did.
type worldTransition struct {
	// from is the root the commit started at.
	from *block.BlockRef
	// to is the root the commit made.
	to *block.BlockRef
	// step is what the commit's writes did, or nil when the commit forfeited
	// its record.
	step *block.ReplacementStep
}

// worldPut is a publication that put World pages: its receipt, each entry
// whose put the store may report found, by index, keeping only its ref and the
// refs it names, and the number of recounts the ledger had asked for when it
// was submitted.
type worldPut struct {
	receipt  *block.PublicationReceipt
	pages    map[int]*block.PutBatchEntry
	recounts uint64
}

// newWorldPut returns the puts of pages among entries, the entries of the
// publication receipt resolves, submitted after recounts recounts were asked
// for. A counted page never reports found.
func newWorldPut(receipt *block.PublicationReceipt, entries, pages []*block.PutBatchEntry, recounts uint64) worldPut {
	// Index the pages, then keep each one the receipt reports by position.
	isPage := make(map[*block.PutBatchEntry]struct{}, len(pages))
	for _, e := range pages {
		isPage[e] = struct{}{}
	}
	kept := make(map[int]*block.PutBatchEntry, len(pages))
	for i, e := range entries {
		if _, ok := isPage[e]; ok && !e.Tombstone && !e.Counted {
			kept[i] = &block.PutBatchEntry{Ref: e.Ref, Refs: e.Refs}
		}
	}
	return worldPut{receipt: receipt, pages: kept, recounts: recounts}
}

// pendingNet is a named World's interval waiting for the puts that stored its
// pages: their receipts report which pages the store already held, which the
// record needs to net. It is used under the writer lock.
//
// A receipt that never completes keeps the interval pending, and the ledger
// stops advancing at its record. The receipt carries no context to end the
// wait, so the ledger orphans the record once maxPendingDeadlists later Worlds
// are named, counting it as stalled; its deaths leak and nothing is freed.
type pendingNet struct {
	// steps are the steps from the last named World to this one, joined only
	// when netting, off the writer lock, or nil when a write on the chain
	// recorded none: the record then nets from the puts alone.
	steps block.ReplacementSteps
	// puts are the publications that put the interval's pages.
	puts []worldPut
	// lineage is the interval that ended at the World this one starts at,
	// whose puts show how the store first stored that World's root, or nil
	// when none was recorded. It is dropped once the encoding starts.
	lineage *pendingNet
	// sealed is set once the naming publication joined puts.
	sealed bool
	// encoded closes once the record is encoded off the writer lock, after
	// which the fields below are set.
	encoded chan struct{}
	// dead is what died in the interval.
	dead *block.Replacements
	// blocks and record are the encoded record's blocks and first block.
	blocks []*block.PutBatchEntry
	record *block.BlockRef
	// err is the encoding error, if any.
	err error
}

// seal adds the naming publication's puts, or none when it was not
// submitted, after which the interval waits only on its receipts.
func (n *pendingNet) seal(put *worldPut) {
	if put != nil {
		n.puts = append(n.puts, *put)
	}
	n.sealed = true
}

// ready reports whether the interval is sealed and each put resolved.
func (n *pendingNet) ready() bool {
	if !n.sealed {
		return false
	}
	for _, put := range n.puts {
		select {
		case <-put.receipt.Done():
		default:
			return false
		}
	}
	return true
}

// poll starts encoding the interval's record as seq from from to to once
// every put resolved, and reports whether the encoded record is ready. The
// ledger has asked for recounts recounts, and recounting is set while it
// waits for one. Netting walks every page the interval wrote, tens of
// milliseconds for a long interval, so it runs off the writer lock and no
// commit waits on it; it waits on nothing, so it always ends. Call it under
// the writer lock.
//
// A record whose from the store's counts may not hold as a holder nets from
// the puts alone, as one without steps does: it kills nothing before the
// interval and takes nothing from from, which leaks the tree at from.
func (n *pendingNet) poll(seq uint64, from, to *block.BlockRef, recounts uint64, recounting bool) bool {
	// Start the encoding once every put, and the lineage's, resolved.
	if n.encoded == nil {
		if !n.ready() || (n.lineage != nil && !n.lineage.ready()) {
			return false
		}
		if n.steps != nil && !from.GetEmpty() && !n.counted(from, recounts, recounting) {
			writeFlows.unfunded.Add(1)
			n.steps = nil
		}
		n.lineage = nil
		n.encoded = make(chan struct{})
		if n.steps == nil {
			from = nil
		}
		go func() {
			defer close(n.encoded)
			n.dead = n.net(to)
			n.blocks, n.record, n.err = block.EncodeDeadlist(seq, from, to, n.dead)
		}()
		return false
	}

	// Report whether it finished.
	select {
	case <-n.encoded:
		return true
	default:
		return false
	}
}

// counted reports whether the store's shared counts hold from, the root the
// interval starts at, with its lineage as a holder: the record that takes from
// is the first holder of the root, the one a count leaves out, and applying it
// takes a count from from.
//
// A sharing walk counts the holders the graph holds and the From of each
// named record, but not the lineage of a root no record names yet, so a walk
// between the root's first put and the naming of its record counts it one
// holder short. The counts hold the lineage only when the first successful put
// of from in the lineage's interval stored it afresh while the counts were
// complete, and the ledger has asked for no recount since that put was
// submitted, nor waits for one, so the record's name reaches the store before
// any walk the ledger asks for begins. A walk the store begins on its own,
// when it opens, restarts the engine and its ledger with it.
//
// The ledger counts a recount once the save asking for it is submitted, after
// any put submitted with it, so a put applied after a recount began may count
// as one submitted before it; that only leaks.
func (n *pendingNet) counted(from *block.BlockRef, recounts uint64, recounting bool) bool {
	// Find the first successful put of from. A page listed twice in one put
	// finds its own first copy, so its first entry decides.
	if n.lineage == nil || recounting {
		return false
	}
	for _, put := range n.lineage.puts {
		if put.receipt.Err() != nil {
			continue
		}
		first := -1
		for i, e := range put.pages {
			if e.Ref.EqualsRef(from) && (first < 0 || i < first) {
				first = i
			}
		}
		if first < 0 {
			continue
		}

		// Require a fresh put while counting, with no recount since.
		existed := put.receipt.Existed()
		found := first < len(existed) && existed[first]
		return !found && put.receipt.Counting() && put.recounts == recounts
	}
	return false
}

// net returns what died in the interval ending at to. A page every successful
// put of it reported the store held is found; one any put stored afresh is not,
// since that put deferred the edges to its children and the record counts
// them. A page no successful put stored is never listed dead, which leaks when
// another path stored it. Without steps, the
// record counts the edges of the pages the successful puts stored afresh and
// kills nothing, as block.NetPuts describes. Call it once ready.
func (n *pendingNet) net(to *block.BlockRef) *block.Replacements {
	// Gather the pages the successful puts carried, and which of them a put
	// stored afresh.
	fresh := make(map[string]bool)
	carried := make(map[string]struct{})
	var pages []*block.PutBatchEntry
	for _, put := range n.puts {
		if put.receipt.Err() != nil {
			continue
		}
		existed := put.receipt.Existed()
		for i, e := range put.pages {
			key := e.Ref.MarshalString()
			if _, ok := carried[key]; !ok {
				carried[key] = struct{}{}
				pages = append(pages, e)
			}
			fresh[key] = fresh[key] || i >= len(existed) || !existed[i]
		}
	}

	// Net the joined steps, or count the puts' edges.
	if n.steps == nil {
		return block.NetPuts(pages, fresh)
	}
	found := make(map[string]bool, len(fresh))
	for key, isFresh := range fresh {
		found[key] = !isFresh
	}
	return n.steps.Join().Net(found)
}

// worldDeaths gathers the World transitions the engine's block engines commit,
// local writes and replay alike, and the publications that put World pages,
// and joins the chain from the last named World to the next into one step.
//
// A local write publishes its World before replay accepts it, and replay may
// rebuild the World in another order, so a transition's record holds only if
// its World is on the accepted chain. Composition walks back from the named
// World through the latest transition into each root, so a fork off the chain
// is never composed, and a chain that returns to an earlier root, such as a
// presence toggling A, B, A, keeps every step: the pages the loop created and
// dropped die in the record. A local fork that itself returns to a root on the
// chain is composed too; its pages were published and no accepted World names
// them, so they die with it. A gap or a forfeited record on the chain forfeits
// the interval: what died in it leaks and is never reported. It is safe for
// concurrent use.
type worldDeaths struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// from is the root the next composition starts at, or nil when the
	// interval is already forfeited.
	from *block.BlockRef
	// fromNet is the interval that ended at from, or nil when none was
	// recorded.
	fromNet *pendingNet
	// trs holds the transitions since the last named World, in the order
	// their commits recorded them.
	trs []worldTransition
	// puts are the publications that put World pages since the last
	// composition.
	puts []worldPut
	// putKeys is the number of pages in puts.
	putKeys int
	// putsDropped is set when puts filled and a publication's pages were
	// dropped, so no record of the interval can count them.
	putsDropped bool
}

// newWorldDeaths starts gathering at from, a World the store holds, or with
// the first interval forfeited when from is nil.
func newWorldDeaths(from *block.BlockRef) *worldDeaths {
	return &worldDeaths{from: from.Clone()}
}

// add records a commit's transition. It is the World block engines'
// TransitionSink.
func (d *worldDeaths) add(from, to *block.BlockRef, step *block.ReplacementStep) {
	// A commit that changed nothing has nothing to compose.
	if from.EqualsRef(to) {
		return
	}

	// Record it, or forfeit the interval when the table is full.
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if len(d.trs) >= maxWorldTransitions {
		d.from, d.fromNet, d.trs = nil, nil, nil
		return
	}
	d.trs = append(d.trs, worldTransition{from: from.Clone(), to: to.Clone(), step: step})
}

// addPut records a publication that put World pages, dropping it when the
// puts are full.
func (d *worldDeaths) addPut(put worldPut) {
	// Skip a publication that put no pages.
	if len(put.pages) == 0 {
		return
	}

	// Add it unless the puts are full.
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if d.putKeys+len(put.pages) > maxWorldPuts {
		d.putsDropped = true
		return
	}
	d.puts = append(d.puts, put)
	d.putKeys += len(put.pages)
}

// take joins the steps from the last named World, from, to the World at root
// with the puts since, and starts the next interval at root. When no chain of
// recorded transitions reaches root, or one on it recorded no step, net has no
// steps and the record nets from the puts alone. net is nil when puts were
// dropped; the interval then forfeits. from is nil when no World was named
// before root, or the transitions filled. Only the later transitions root
// reaches stay.
func (d *worldDeaths) take(root *block.BlockRef) (from *block.BlockRef, net *pendingNet) {
	// Walk the chain back from root and join it.
	d.mtx.Lock()
	defer d.mtx.Unlock()
	steps, last := d.compose(root)
	from = d.from
	if !d.putsDropped {
		net = &pendingNet{steps: steps, puts: d.puts, lineage: d.fromNet}
	}

	// Keep the later transitions root reaches and start there.
	d.prune(root, last)
	d.from, d.fromNet = root.Clone(), net
	d.puts, d.putKeys, d.putsDropped = nil, 0, false
	return from, net
}

// compose walks back from root through the latest earlier transition into
// each root until none is left, and returns the steps in order, or nil when
// the walk does not end at the last named World or crosses a forfeited
// commit. last is the index after the latest transition into root, or zero
// when none is recorded. Assumes mtx is locked.
func (d *worldDeaths) compose(root *block.BlockRef) (steps block.ReplacementSteps, last int) {
	// Index the transitions by the root they made, oldest first.
	into := make(map[string][]int, len(d.trs))
	for i, tr := range d.trs {
		key := tr.to.MarshalString()
		into[key] = append(into[key], i)
	}

	// Walk back from root, each time through the latest earlier transition
	// into the root reached so far.
	cur, end, forfeit := root, len(d.trs), false
	for {
		idx := into[cur.MarshalString()]
		n, _ := slices.BinarySearch(idx, end)
		if n == 0 {
			break
		}
		i := idx[n-1]
		if last == 0 {
			last = i + 1
		}
		tr := d.trs[i]
		forfeit = forfeit || tr.step == nil
		steps = append(steps, tr.step)
		cur, end = tr.from, i
	}

	// Join only a whole chain from the last named World.
	if forfeit || d.from == nil || !cur.EqualsRef(d.from) {
		return nil, last
	}
	if len(steps) == 0 {
		// An unchanged World lost nothing.
		return block.ReplacementSteps{{From: d.from, To: root}}, last
	}
	slices.Reverse(steps)
	return steps, last
}

// prune keeps the transitions from index last on that root reaches, in order.
// Assumes mtx is locked.
func (d *worldDeaths) prune(root *block.BlockRef, last int) {
	reached := map[string]struct{}{root.MarshalString(): {}}
	var kept []worldTransition
	for _, tr := range d.trs[last:] {
		if _, ok := reached[tr.from.MarshalString()]; ok {
			kept = append(kept, tr)
			reached[tr.to.MarshalString()] = struct{}{}
		}
	}
	d.trs = kept
}

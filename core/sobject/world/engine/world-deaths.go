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
// compositions. Past it the puts are dropped, so the pages they found count as
// created, which leaks.
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

// worldPut is a publication that put World pages: its receipt, and the key of
// each entry whose put the store may report found, by index.
type worldPut struct {
	receipt *block.PublicationReceipt
	keys    map[int]string
}

// newWorldPut returns the puts of pages among entries, the entries of the
// publication receipt resolves. A counted page never reports found.
func newWorldPut(receipt *block.PublicationReceipt, entries, pages []*block.PutBatchEntry) worldPut {
	// Index the pages, then key each one the receipt reports by position.
	isPage := make(map[*block.PutBatchEntry]struct{}, len(pages))
	for _, e := range pages {
		isPage[e] = struct{}{}
	}
	keys := make(map[int]string, len(pages))
	for i, e := range entries {
		if _, ok := isPage[e]; ok && !e.Tombstone && !e.Counted {
			keys[i] = e.Ref.MarshalString()
		}
	}
	return worldPut{receipt: receipt, keys: keys}
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
	// when netting, off the writer lock.
	steps block.ReplacementSteps
	// puts are the publications that put the interval's pages.
	puts []worldPut
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
// every put resolved, and reports whether the encoded record is ready. Netting
// walks every page the interval wrote, tens of milliseconds for a long
// interval, so it runs off the writer lock and no commit waits on it; it
// waits on nothing, so it always ends. Call it under the writer lock.
func (n *pendingNet) poll(seq uint64, from, to *block.BlockRef) bool {
	// Start the encoding once every put resolved.
	if n.encoded == nil {
		if !n.ready() {
			return false
		}
		n.encoded = make(chan struct{})
		go func() {
			defer close(n.encoded)
			n.dead = n.net()
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

// net returns what died in the interval. A page a successful put reported the
// store held is found; a page no successful put stored is never listed dead,
// which leaks when another path stored it. Call it once ready.
func (n *pendingNet) net() *block.Replacements {
	puts := make(map[string]bool)
	for _, put := range n.puts {
		if put.receipt.Err() != nil {
			continue
		}
		existed := put.receipt.Existed()
		for i, key := range put.keys {
			puts[key] = puts[key] || (i < len(existed) && existed[i])
		}
	}
	return n.steps.Join().Net(puts)
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
	// trs holds the transitions since the last named World, in the order
	// their commits recorded them.
	trs []worldTransition
	// puts are the publications that put World pages since the last
	// composition.
	puts []worldPut
	// putKeys is the number of keys in puts.
	putKeys int
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
		d.from, d.trs = nil, nil
		return
	}
	d.trs = append(d.trs, worldTransition{from: from.Clone(), to: to.Clone(), step: step})
}

// addPut records a publication that put World pages, dropping it when the
// puts are full.
func (d *worldDeaths) addPut(put worldPut) {
	// Skip a publication that put no pages.
	if len(put.keys) == 0 {
		return
	}

	// Add it unless the puts are full.
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if d.putKeys+len(put.keys) > maxWorldPuts {
		return
	}
	d.puts = append(d.puts, put)
	d.putKeys += len(put.keys)
}

// take joins the steps from the last named World, from, to the World at root
// with the puts since, and starts the next interval at root. net is nil when
// no chain of recorded transitions reaches root or one on it forfeited; the
// interval then leaks. from is nil when the interval forfeited before root was
// named. Only the later transitions root reaches stay.
func (d *worldDeaths) take(root *block.BlockRef) (from *block.BlockRef, net *pendingNet) {
	// Walk the chain back from root and join it.
	d.mtx.Lock()
	defer d.mtx.Unlock()
	steps, last := d.compose(root)
	from = d.from
	if from != nil && steps != nil {
		net = &pendingNet{steps: steps, puts: d.puts}
	}

	// Keep the later transitions root reaches and start there.
	d.prune(root, last)
	d.from = root.Clone()
	d.puts = nil
	d.putKeys = 0
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

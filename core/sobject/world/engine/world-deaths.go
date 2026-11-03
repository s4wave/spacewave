package sobject_world_engine

import (
	"context"
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
	// receipt reports the actual publication's outcome by entry position.
	receipt *block.PublicationReceipt
	// pages retain immutable page metadata in ascending publication position.
	pages []worldPutPage
	// recounts is the ledger's recount counter when the put was submitted.
	recounts uint64
}

// worldPutPage retains one page's metadata and position in its publication.
type worldPutPage struct {
	// index addresses the entry's outcome in the publication receipt.
	index int
	// entry retains only the immutable reference and child references.
	entry *block.PutBatchEntry
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
	kept := make([]worldPutPage, 0, len(pages))
	for i, e := range entries {
		if _, ok := isPage[e]; ok && !e.Tombstone && !e.Counted {
			kept = append(kept, worldPutPage{index: i, entry: &block.PutBatchEntry{Ref: e.Ref, Refs: e.Refs}})
		}
	}
	return worldPut{receipt: receipt, pages: kept, recounts: recounts}
}

// markNetted returns entries with each unmarked page among them replaced by a
// copy marked Netted, and the copies, which newWorldPut takes as the pages.
// The Volume leaves the edges between Netted entries to the interval's record,
// so only a page whose receipt reaches a record may be marked: the record
// counts the edges and lists each put that found the page in Found. It copies,
// as MarkOriginal does, since a kept page may be in a publication in flight.
func markNetted(entries, pages []*block.PutBatchEntry) (marked, netted []*block.PutBatchEntry) {
	// Index the pages.
	isPage := make(map[*block.PutBatchEntry]struct{}, len(pages))
	for _, e := range pages {
		isPage[e] = struct{}{}
	}

	// Replace each unmarked page with a marked copy.
	marked = slices.Clone(entries)
	for i, e := range marked {
		if _, ok := isPage[e]; ok && !e.Tombstone && !e.Counted {
			c := *e
			c.Netted = true
			marked[i] = &c
			netted = append(netted, &c)
		}
	}
	return marked, netted
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
	// cancelEncoding stops off-writer receipt waits when the entry is abandoned.
	cancelEncoding context.CancelFunc
	// encoded closes once the record is encoded off the writer lock, after
	// which the fields below are set.
	encoded chan struct{}
	// preparation is the writer-owned stage job after encoding completes.
	preparation *deadlistPreparation
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

// poll starts encoding the interval's record as seq from from to to once
// its puts are sealed, and reports whether the encoded record is ready. The
// ledger has asked for recounts recounts, and recounting is set while it
// waits for one. Netting walks every page the interval wrote, tens of
// milliseconds for a long interval, so it runs off the writer lock and no
// commit waits on it. Receipt waits cancel with the entry or engine. Call it under
// the writer lock.
//
// A record whose from the store's counts may not hold as a holder nets from
// the puts alone, as one without steps does: it kills nothing before the
// interval and takes nothing from from, which leaks the tree at from.
func (n *pendingNet) poll(ctx context.Context, seq uint64, from, to *block.BlockRef, recounts uint64, recounting bool, notify chan<- struct{}) bool {
	// Start one background task once both receipt lists are immutable.
	if n.encoded == nil {
		if !n.sealed || (n.lineage != nil && !n.lineage.sealed) {
			return false
		}
		n.encoded = make(chan struct{})
		ctx, n.cancelEncoding = context.WithCancel(ctx)
		go func() {
			// Detach completed receipt waits from the engine context.
			defer n.cancelEncoding()

			// Publish the immutable encoding after every receipt scan completes.
			defer func() {
				close(n.encoded)
				select {
				case notify <- struct{}{}:
				default:
				}
			}()

			// Receipt waits use immutable sealed inputs off the writer. An
			// abandoned interval cancels the wait even if a receipt never settles.
			puts := [][]worldPut{n.puts}
			if n.lineage != nil {
				puts = append(puts, n.lineage.puts)
			}
			for _, list := range puts {
				for _, put := range list {
					select {
					case <-put.receipt.Done():
					case <-ctx.Done():
						n.err = ctx.Err()
						return
					}
				}
			}
			n.encode(seq, from, to, recounts, recounting)
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

// encode certifies and nets only immutable, completed receipt inputs. It does
// no I/O and waits on nothing; its caller publishes the result through encoded.
func (n *pendingNet) encode(seq uint64, from, to *block.BlockRef, recounts uint64, recounting bool) {
	// An uncertified lineage cannot consume any historical holder.
	if n.steps != nil && !from.GetEmpty() && !n.counted(from, recounts, recounting) {
		writeFlows.unfunded.Add(1)
		n.steps = nil
	}
	n.lineage = nil
	if n.steps == nil {
		from = nil
	}

	// Encode the netted interval as bounded physical record blocks.
	n.dead = n.net(to)
	n.blocks, n.record, n.err = block.EncodeDeadlist(seq, from, to, n.dead)
}

// counted reports whether the store's shared counts hold from, the root the
// interval starts at, with its lineage as a holder: the record that takes from
// is the first holder of the root, the one a count leaves out, and applying it
// takes a count from from.
//
// A sharing walk rebuilds the actual stored graph and discards an interval's
// provisional lineage and historical claims. Only a fresh first root put in
// the unchanged complete basis can certify that lineage. Naming a precise
// record later transfers its temporary From holder atomically; an uncertain
// record abandons all historical refunds. Another engine's recount or physical
// erasure is visible through the receipts' Volume generation, independently of
// this ledger's recount counter. Naming and application check that basis again.
//
// The ledger counts a recount once the save asking for it is submitted, after
// any put submitted with it, so a put applied after a recount began may count
// as one submitted before it; that only leaks.
func (n *pendingNet) counted(from *block.BlockRef, recounts uint64, recounting bool) bool {
	// Find the first successful put of from. A page listed twice in one put
	// finds its own first copy, so its first entry decides.
	complete, generation := n.countingBasis()
	if n.lineage == nil || recounting || !complete {
		return false
	}
	for _, put := range n.lineage.puts {
		if put.receipt.Err() != nil {
			continue
		}
		first := -1
		for _, page := range put.pages {
			if page.entry.Ref.EqualsRef(from) {
				first = page.index
				break
			}
		}
		if first < 0 {
			continue
		}

		// Require a fresh put while counting, with no recount since.
		existed := put.receipt.Existed()
		found := first < len(existed) && existed[first]
		return !found && put.receipt.Counting() && put.receipt.CountingGeneration() == generation && put.recounts == recounts
	}
	return false
}

// net returns what died in the interval ending at to. A page every successful
// put of it reported the store held is found; one any put stored afresh is not,
// since that put deferred the edges to its children and the record counts
// them. A page no successful put stored is never listed dead, which leaks when
// another path stored it. Without steps, the record counts the edges of the
// pages the successful puts stored afresh and kills nothing, as block.NetPuts
// describes. Either way the record lists in Found each successful put that
// found a page, whose provisional count remains through child funding. It
// also records the complete generation and a conservative NetPuts summary,
// computed off the writer, for a store whose recount erased that basis. Call
// it once ready.
func (n *pendingNet) net(to *block.BlockRef) *block.Replacements {
	// Gather the pages the successful puts carried, which of them a put
	// stored afresh, and each put that found one. A page listed twice in one
	// put finds its own first copy, so its first entry decides.
	fresh := make(map[string]bool)
	carried := make(map[string]struct{})
	var pages, foundPuts []*block.PutBatchEntry
	for _, put := range n.puts {
		if put.receipt.Err() != nil {
			continue
		}
		existed := put.receipt.Existed()
		for _, page := range put.pages {
			i, e := page.index, page.entry
			key := e.Ref.MarshalString()
			if _, ok := carried[key]; !ok {
				carried[key] = struct{}{}
				pages = append(pages, e)
			}
			isFresh := i >= len(existed) || !existed[i]
			fresh[key] = fresh[key] || isFresh
		}
		foundPuts = append(foundPuts, put.found(existed)...)
	}

	// Net the joined steps, or count the puts' edges.
	var dead *block.Replacements
	if n.steps == nil {
		dead = block.NetPuts(pages, fresh)
	} else {
		found := make(map[string]bool, len(fresh))
		for key, isFresh := range fresh {
			found[key] = !isFresh
		}
		step := n.steps.Join()
		step.AddFresh(pages, fresh)
		dead = step.Net(found)
	}
	for _, e := range foundPuts {
		dead.Found = append(dead.Found, e.Ref)
	}
	dead.CountsComplete, dead.CountingGeneration = n.countingBasis()
	dead.FallbackHeld = block.NetPuts(pages, fresh).Held
	return dead
}

// countingBasis certifies all successful puts against one Volume generation.
// Another engine's recount is visible through receipts too; the ledger's own
// recount counter alone cannot certify holders across that boundary.
func (n *pendingNet) countingBasis() (bool, uint64) {
	first := true
	var generation uint64
	for _, put := range n.puts {
		if put.receipt.Err() != nil {
			continue
		}
		if !put.receipt.Counting() {
			return false, 0
		}
		if first {
			generation, first = put.receipt.CountingGeneration(), false
		} else if generation != put.receipt.CountingGeneration() {
			return false, 0
		}
	}
	return !first, generation
}

// found returns each page the put found stored, by its first entry, as the
// Volume counts it.
func (p *worldPut) found(existed []bool) []*block.PutBatchEntry {
	// Ascending receipt positions make the first occurrence authoritative.
	seen := make(map[string]struct{}, len(p.pages))
	var out []*block.PutBatchEntry
	for _, page := range p.pages {
		key := page.entry.Ref.MarshalString()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		// Only the first occurrence can return this publication's hold.
		if page.index < len(existed) && existed[page.index] {
			out = append(out, page.entry)
		}
	}
	return out
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

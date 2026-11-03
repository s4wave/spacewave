package block

import "maps"

// ReplacementStep is what the writes of one or more intervals did to the
// stored pages under a root, before netting. Steps along a chain of roots join
// into one, so a page one step creates and a later step replaces nets out.
type ReplacementStep struct {
	// From is the root the step began at.
	From *BlockRef
	// To is the root the step ended at.
	To *BlockRef
	// replaced holds each stored page a write encoded again, by ref.
	replaced map[string]pageBase
	// created holds each page a write stored, by ref.
	created map[string]pageBase
	// original holds each replaced page that no earlier write in the step
	// created, by ref: a page of the tree at From.
	original map[string]*BlockRef
}

// NetPuts returns the record of an interval whose writes recorded no steps,
// from pages, each page its successful puts carried. fresh holds, by
// MarshalString, each page a put stored afresh.
//
// The record lists no page dead. The tree before the interval, and each page
// the interval stored and dropped, falls when its own holder releases it,
// which releases each child it names: a record that also killed those pages
// would hand their children to new parents a second time. So the record
// counts the edges alone: a page stored afresh adds an edge to each child it
// names, which no put counted, and a child stored afresh takes its first
// holder without a count. The record holds a child once for each such edge,
// less that first holder.
func NetPuts(pages []*PutBatchEntry, fresh map[string]bool) *Replacements {
	// Count the edges from each page stored afresh, in page order.
	edges := make(map[string]int)
	var children []*BlockRef
	for _, e := range pages {
		if !fresh[e.Ref.MarshalString()] {
			continue
		}
		named := make(map[string]struct{}, len(e.Refs))
		for _, ref := range e.Refs {
			key := ref.MarshalString()
			if _, dup := named[key]; dup || ref.GetEmpty() {
				continue
			}
			named[key] = struct{}{}
			if edges[key] == 0 {
				children = append(children, ref)
			}
			edges[key]++
		}
	}

	// Hold each child once per edge, less a fresh child's first holder.
	out := &Replacements{}
	for _, ref := range children {
		n := edges[ref.MarshalString()]
		if fresh[ref.MarshalString()] {
			n--
		}
		for range n {
			out.Held = append(out.Held, ref)
		}
	}
	return out
}

// ReplacementSteps are the steps of a chain of roots, each starting where the
// one before ended.
type ReplacementSteps []*ReplacementStep

// Original reports whether ref is a page of the tree at the first root that a
// step replaced: some step replaced it before any step created it. It costs a
// lookup per step, which suits a few refs under a lock; Join builds the set.
func (s ReplacementSteps) Original(ref *BlockRef) bool {
	key := ref.MarshalString()
	for _, step := range s {
		if _, ok := step.original[key]; ok {
			return true
		}
		if _, ok := step.created[key]; ok {
			return false
		}
	}
	return false
}

// MarkOriginal replaces each entry that puts an original page with a counted
// copy: the tree at the first root already holds the page, so its put adds no
// holder. The first root is the exception: no page of the tree names it, only
// holds outside the tree such as a replay span block, so each put that names
// it counts.
func (s ReplacementSteps) MarkOriginal(entries []*PutBatchEntry) {
	if len(s) == 0 {
		return
	}
	from := s[0].From
	for i, e := range entries {
		if !e.Tombstone && !e.Counted && !e.Ref.EqualsRef(from) && s.Original(e.Ref) {
			counted := *e
			counted.Counted = true
			entries[i] = &counted
		}
	}
}

// Join joins the steps, at least one, into one step from the first root to
// the last.
//
// A replaced page is original when no earlier step created it. The tree at
// the first root reaches every page later steps replace that no earlier step
// created, since each step only adds the pages it creates, so an original page
// is a page of that tree.
func (s ReplacementSteps) Join() *ReplacementStep {
	out := &ReplacementStep{
		From:     s[0].From.Clone(),
		To:       s[len(s)-1].To.Clone(),
		replaced: make(map[string]pageBase),
		created:  make(map[string]pageBase),
		original: make(map[string]*BlockRef),
	}
	for _, step := range s {
		for key, ref := range step.original {
			if _, ok := out.created[key]; !ok {
				out.original[key] = ref
			}
		}
		maps.Copy(out.replaced, step.replaced)
		maps.Copy(out.created, step.created)
	}
	return out
}

// AddFresh adds to the step, as created pages, each of pages a put stored
// afresh that the step neither created nor replaced. pages are the pages the
// interval's successful puts carried, and fresh holds, by MarshalString, each
// one a put stored afresh. Such a page was written outside the step's chain,
// as by a commit the replay later ordered away or a replayed position a later
// replay dropped. Its put left the edges to its children to the record, so Net
// must see it: a live one holds its children, and a dead one is dropped.
func (s *ReplacementStep) AddFresh(pages []*PutBatchEntry, fresh map[string]bool) {
	for _, e := range pages {
		key := e.Ref.MarshalString()
		_, isCreated := s.created[key]
		_, isReplaced := s.replaced[key]
		if fresh[key] && !isCreated && !isReplaced {
			s.created[key] = pageBase{ref: e.Ref, refs: e.Refs}
		}
	}
}

// Net derives what the step did to the stored pages. puts holds, by
// MarshalString, each page a put carried, true when the put found the store
// already held it; nil means the step's puts stored every page it created
// afresh. A created page no put carried is the stored page: a live one is
// stored, since the tree under the root is, and a dead one is never listed.
// An original page is never found: its put was counted.
//
// The live pages are those To reaches through the pages the step wrote:
// created, found or replaced. A page outside them is a stored subtree the step
// did not touch, live as a whole. Only a page the step stored afresh adds new
// edges to its children: a found or original page is the stored page, whose
// edges were counted when it was first stored.
//
// The candidates are the old root, every replaced page that may be stored and
// every page one not stored afresh named. A replaced original page is listed dead only when the
// dead old root reaches it through dead original pages: a page dirtied
// through a parent the step dropped may still sit under a parent no step
// touched, which holds it, so the record leaves it and its children alone. A
// dead page the step created is listed only when a put stored it: a page
// created and dropped in memory never reached the store, and a later write may
// store the same page afresh, which the listing would free. Every other dead
// candidate heads a dead subtree, written or not, since a stored page's
// children are stored. A dead page the step stored afresh is listed in
// Dropped: no put counted its edges, so its children are not candidates.
//
// A live page that is neither root is held once for each page the step stored
// afresh, or the new root, that names it: each such edge is a holder no put
// counted. A page the step stored afresh takes its first holder without a
// count and is held once for each other. A page whose old parent died moved,
// and applying the record takes that parent's count back when it frees, which
// only applying can see; a page a write copied under two parents keeps one
// more holder.
//
// A live page existed when it had a holder before the record: the step found
// it, or did not store it afresh and no page it stored afresh, nor the new
// root, names it. Applying the record takes one count from it when its listed
// parent frees, and from the old root for the lineage.
func (s *ReplacementStep) Net(puts map[string]bool) *Replacements {
	// fresh reports whether a put of the step stored the page under key
	// afresh, and found whether the step created the page but found it stored.
	fresh := func(key string) bool {
		_, isCreated := s.created[key]
		_, isOriginal := s.original[key]
		found, carried := puts[key]
		return isCreated && !isOriginal && (puts == nil || (carried && !found))
	}
	found := func(key string) bool {
		_, isCreated := s.created[key]
		_, isOriginal := s.original[key]
		return isCreated && !isOriginal && !fresh(key)
	}

	// Walk the live pages from the new root through the written pages,
	// counting the fresh pages that name each page, and the new root.
	type step struct {
		ref    *BlockRef
		direct bool
	}
	live := make(map[string]*BlockRef)
	direct := make(map[string]int)
	stk := []step{{s.To, true}}
	for len(stk) != 0 {
		st := stk[len(stk)-1]
		stk = stk[:len(stk)-1]
		if st.ref.GetEmpty() {
			continue
		}
		key := st.ref.MarshalString()
		if st.direct {
			direct[key]++
		}
		if _, ok := live[key]; ok {
			continue
		}
		live[key] = st.ref
		page, ok := s.created[key]
		if !ok {
			page, ok = s.replaced[key]
		}
		if !ok {
			continue
		}
		isFresh := fresh(key)
		seen := make(map[string]struct{}, len(page.refs))
		for _, ref := range page.refs {
			if _, dup := seen[ref.MarshalString()]; !dup {
				seen[ref.MarshalString()] = struct{}{}
				stk = append(stk, step{ref, isFresh})
			}
		}
	}

	// Find the dead original pages the old root reaches through dead
	// original pages. The applier reads their child lists from the store.
	dead := make(map[string]struct{})
	if _, isLive := live[s.From.MarshalString()]; !isLive && !s.From.GetEmpty() {
		stk := []string{s.From.MarshalString()}
		for len(stk) != 0 {
			key := stk[len(stk)-1]
			stk = stk[:len(stk)-1]
			if _, ok := dead[key]; ok {
				continue
			}
			dead[key] = struct{}{}
			for _, ref := range s.replaced[key].refs {
				child := ref.MarshalString()
				_, isOriginal := s.original[child]
				_, isLive := live[child]
				if isOriginal && !isLive {
					stk = append(stk, child)
				}
			}
		}
	}

	// List the dead replaced pages, and gather the pages each stored one
	// named. A dead original page the old root does not reach is untouched:
	// a stored parent outside the step holds it. A replaced page no put
	// stored counted no holder of the pages it named, and a dead one stored
	// afresh is dropped, its edges never counted.
	named := make(map[string]*BlockRef)
	listed := make(map[string]struct{})
	readNamed := make(map[string]struct{})
	out := &Replacements{}
	for key, page := range s.replaced {
		_, isLive := live[key]
		_, isOriginal := s.original[key]
		_, isDead := dead[key]
		if !s.stored(puts, key) || (isOriginal && !isLive && !isDead) {
			continue
		}
		if !isLive && found(key) {
			continue
		}
		if !isLive {
			listed[key] = struct{}{}
			if fresh(key) {
				out.Dropped = append(out.Dropped, page.ref)
				continue
			}
			out.Replaced = append(out.Replaced, page.ref)
		}
		for _, ref := range page.refs {
			named[ref.MarshalString()] = ref
			if !isLive && isOriginal {
				readNamed[ref.MarshalString()] = struct{}{}
			}
		}
	}
	for key, page := range s.created {
		_, isLive := live[key]
		_, isReplaced := s.replaced[key]
		if !isLive && !isReplaced && s.stored(puts, key) && !found(key) {
			listed[key] = struct{}{}
			if fresh(key) {
				out.Dropped = append(out.Dropped, page.ref)
			} else {
				out.Replaced = append(out.Replaced, page.ref)
			}
		}
	}

	// Remove every other dead candidate but the untouched pages.
	if !s.From.GetEmpty() {
		named[s.From.MarshalString()] = s.From
	}
	for key, ref := range named {
		_, isLive := live[key]
		_, isListed := listed[key]
		_, isOriginal := s.original[key]
		_, isDead := dead[key]
		_, isReplaced := s.replaced[key]
		untouched := isReplaced && isOriginal && !isLive && !isDead
		if !isLive && !isListed && !untouched {
			out.Removed = append(out.Removed, ref)
		}
	}

	// Keep the live pages that no dead original page named: attached here
	// from a position outside the touched pages, or written by this step. A
	// written page may equal a stored page inside a removed subtree.
	//
	// Hold each live page once per fresh page naming it, whose edge no put
	// counted. A dead page's edge to it is taken back when the record frees
	// that page, so a page that only moved between parents nets nothing, and
	// a page a write copied under two parents nets one. A page stored afresh
	// takes its first holder without a count, so it is held for each other
	// fresh page naming it. List the live pages that had a holder before.
	from, to := s.From.MarshalString(), s.To.MarshalString()
	for key, ref := range live {
		if key != from && key != to {
			n := direct[key]
			if fresh(key) {
				n = max(n-1, 0)
			}
			for range n {
				out.Held = append(out.Held, ref)
			}
		}
		if _, isNamed := readNamed[key]; !isNamed {
			out.Kept = append(out.Kept, ref)
		}
		if found(key) || (direct[key] == 0 && !fresh(key)) {
			out.Existed = append(out.Existed, ref)
		}
	}
	return out
}

// stored reports whether the page under key, which the step wrote, may be
// stored: an original page, or one a put stored. nil puts stored every page.
func (s *ReplacementStep) stored(puts map[string]bool, key string) bool {
	if _, isOriginal := s.original[key]; isOriginal || puts == nil {
		return true
	}
	_, ok := puts[key]
	return ok
}

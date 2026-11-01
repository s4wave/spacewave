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
// names, which no put counted unless the child shared its batch, and a child
// stored afresh takes its first holder without a count. The record holds a
// child once for each such edge, less that first holder. A found child whose
// put counted a parent in its batch is held for that parent again, which
// leaks.
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

// Net derives what the step did to the stored pages. puts holds, by
// MarshalString, each page a put stored, true when the put found the store
// already held it and counted its holders there; nil means the step's puts
// stored every page it created afresh. An original page is never found: its
// put was counted.
//
// The live pages are those To reaches through the pages the step wrote:
// created, found or replaced. A page outside them is a stored subtree the step
// did not touch, live as a whole. The candidates are the old root, every
// replaced page and every page one named. A candidate not live is dead: a
// replaced or created page has its children accounted, any other heads a dead
// subtree. A dead page the step created is listed only when a put stored it: a
// page created and dropped in memory never reached the store, and a later write
// may store the same page afresh, which the listing would free.
//
// A live page that was not put and is neither root is held once for each
// written page, or the new root, that names it: each such edge is a holder no
// put counted. A page whose old parent died moved, and applying the record
// takes that parent's count back when it frees, which only applying can see;
// a page a write copied under two parents keeps one more holder. A found
// page's holders were counted at its put, and its children gained none, since
// the stored copy already named them.
//
// A live page existed when its edge from the new tree was counted already: a
// put found it, or no created or replaced page and not the new root names it
// and the step did not create it. Applying the record takes one count from it
// when its listed parent frees, and from the old root for the lineage.
func (s *ReplacementStep) Net(puts map[string]bool) *Replacements {
	// Split the found pages from the created ones.
	created := s.created
	var foundPages map[string]pageBase
	for key, page := range s.created {
		isFound := puts[key]
		_, isOriginal := s.original[key]
		if !isFound || isOriginal {
			continue
		}
		if foundPages == nil {
			created = maps.Clone(s.created)
			foundPages = make(map[string]pageBase)
		}
		foundPages[key] = page
		delete(created, key)
	}

	// Walk the live pages from the new root through the written pages,
	// counting the written pages that name each page, and the new root.
	type step struct {
		ref    *BlockRef
		direct bool
	}
	live := make(map[string]*BlockRef)
	direct := make(map[string]int)
	stk := []step{{s.To, true}}
	push := func(page pageBase, direct bool) {
		seen := make(map[string]struct{}, len(page.refs))
		for _, ref := range page.refs {
			key := ref.MarshalString()
			if _, dup := seen[key]; !dup {
				seen[key] = struct{}{}
				stk = append(stk, step{ref, direct})
			}
		}
	}
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
		if page, ok := created[key]; ok {
			push(page, true)
		} else if page, ok := s.replaced[key]; ok {
			push(page, true)
		} else if page, ok := foundPages[key]; ok {
			push(page, false)
		}
	}

	// Gather the pages a stored replaced page named, and those a dead original
	// page named. The applier reads an original page's child list from the
	// store, so an expansion cannot reach its children under the one parent
	// rule. A page the interval created may never have been stored, and a
	// replaced page the new tree reaches is not listed, so neither child list
	// is read. A replaced page no put stored counted no holder of the pages it
	// named, so they are no candidates through it.
	named := make(map[string]*BlockRef)
	readNamed := make(map[string]struct{})
	out := &Replacements{}
	for key, page := range s.replaced {
		_, isLive := live[key]
		_, isOriginal := s.original[key]
		if !s.stored(puts, key) {
			continue
		}
		if !isLive {
			out.Replaced = append(out.Replaced, page.ref)
		}
		for _, ref := range page.refs {
			named[ref.MarshalString()] = ref
			if !isLive && isOriginal {
				readNamed[ref.MarshalString()] = struct{}{}
			}
		}
	}

	// Sort the dead candidates by whether their children are accounted.
	for key, page := range s.created {
		_, isLive := live[key]
		_, isReplaced := s.replaced[key]
		if !isLive && !isReplaced && s.stored(puts, key) {
			out.Replaced = append(out.Replaced, page.ref)
		}
	}
	removed := make([]*BlockRef, 0, len(named)+1)
	for _, ref := range named {
		removed = append(removed, ref)
	}
	if !s.From.GetEmpty() {
		removed = append(removed, s.From)
	}
	seen := make(map[string]struct{}, len(removed))
	for _, ref := range removed {
		key := ref.MarshalString()
		_, isSeen := seen[key]
		_, isLive := live[key]
		_, isReplaced := s.replaced[key]
		_, isCreated := s.created[key]
		seen[key] = struct{}{}
		if !isSeen && !isLive && !isReplaced && !isCreated {
			out.Removed = append(out.Removed, ref)
		}
	}

	// Keep the live pages that no dead original page named: attached here
	// from a position outside the touched pages, or written by this step. A
	// written page may equal a stored page inside a removed subtree.
	//
	// Hold each stored page once per written page naming it, whose edge no put
	// counted. A dead page's edge to it is taken back when the record frees
	// that page, so a page that only moved between parents nets nothing, and a
	// page a write copied under two parents nets one.
	from, to := s.From.MarshalString(), s.To.MarshalString()
	for key, ref := range live {
		_, isReplaced := s.replaced[key]
		_, isWritten := s.created[key]
		if !isReplaced && !isWritten && key != from && key != to {
			for range direct[key] {
				out.Held = append(out.Held, ref)
			}
		}
		if _, isNamed := readNamed[key]; !isNamed {
			out.Kept = append(out.Kept, ref)
		}
	}

	// List the live pages whose edge from the new tree existed.
	for key, ref := range live {
		_, isFound := foundPages[key]
		isDirect := direct[key] != 0
		_, isCreated := created[key]
		if isFound || (!isDirect && !isCreated) {
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

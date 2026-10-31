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
// holder.
func (s ReplacementSteps) MarkOriginal(entries []*PutBatchEntry) {
	for i, e := range entries {
		if !e.Tombstone && !e.Counted && s.Original(e.Ref) {
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
// A live page that was not put, that no replaced page named, and that is not
// the old root, is held when the new root or a created page names it: it
// gained a holder unless its old parent died untouched, which only applying
// the record can see. A found page's holders were counted at its put, and its
// children gained none, since the stored copy already named them.
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
	// marking each page the new root or a created page names.
	type step struct {
		ref    *BlockRef
		direct bool
	}
	live := make(map[string]*BlockRef)
	direct := make(map[string]struct{})
	stk := []step{{s.To, true}}
	for len(stk) != 0 {
		st := stk[len(stk)-1]
		stk = stk[:len(stk)-1]
		if st.ref.GetEmpty() {
			continue
		}
		key := st.ref.MarshalString()
		if st.direct {
			direct[key] = struct{}{}
		}
		if _, ok := live[key]; ok {
			continue
		}
		live[key] = st.ref
		if page, ok := created[key]; ok {
			for _, ref := range page.refs {
				stk = append(stk, step{ref, true})
			}
		} else if page, ok := s.replaced[key]; ok {
			for _, ref := range page.refs {
				stk = append(stk, step{ref, true})
			}
		} else if page, ok := foundPages[key]; ok {
			for _, ref := range page.refs {
				stk = append(stk, step{ref, false})
			}
		}
	}

	// Gather the pages a replaced page named, and those a dead original page
	// named. The applier reads an original page's child list from the store,
	// so an expansion cannot reach its children under the one parent rule. A
	// page the interval created may never have been stored, and a replaced
	// page the new tree reaches is not listed, so neither child list is read.
	named := make(map[string]*BlockRef)
	readNamed := make(map[string]struct{})
	out := &Replacements{}
	for key, page := range s.replaced {
		_, isLive := live[key]
		_, isOriginal := s.original[key]
		if !isLive && (isOriginal || s.stored(puts, key)) {
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
	from := s.From.MarshalString()
	for key, ref := range live {
		if _, isNamed := readNamed[key]; isNamed {
			continue
		}
		out.Kept = append(out.Kept, ref)
		_, isDirect := direct[key]
		_, isReplaced := s.replaced[key]
		_, isWritten := s.created[key]
		if isDirect && !isReplaced && !isWritten && key != from {
			out.Held = append(out.Held, ref)
		}
	}

	// List the live pages whose edge from the new tree existed.
	for key, ref := range live {
		_, isFound := foundPages[key]
		_, isDirect := direct[key]
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

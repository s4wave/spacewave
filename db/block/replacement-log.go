package block

import "sync"

// pageBase is a stored page and the refs it names.
type pageBase struct {
	ref  *BlockRef
	refs []*BlockRef
}

// ReplacementLog gathers the stored pages that writes replaced and created
// between Take calls. Any number of transactions over one store may record into
// it, such as a World's own transaction and the object transactions it stages,
// so a page one writes and another names nets out. It is safe for concurrent
// use.
type ReplacementLog struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// step is the interval in progress.
	step ReplacementStep
	// forfeit is set when the interval's record cannot be trusted.
	forfeit bool
}

// NewReplacementLog starts a log whose first interval begins at the root from.
func NewReplacementLog(from *BlockRef) *ReplacementLog {
	l := &ReplacementLog{}
	l.reset(from)
	return l
}

// From returns the root the current interval began at.
func (l *ReplacementLog) From() *BlockRef {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.step.From.Clone()
}

// Forfeit gives up the current interval's record, for a write the record
// cannot account for, such as one that gives a page a second parent the record
// does not count. Take then reports nothing for the interval, so what died in
// it leaks rather than a page another parent names being reported dead.
func (l *ReplacementLog) Forfeit() {
	l.mtx.Lock()
	l.forfeit = true
	l.mtx.Unlock()
}

// Take returns what the writes since the interval began did to the stored
// pages under its root, with the root now at to, and starts the next interval
// at to. Returns nil when the interval was forfeited.
func (l *ReplacementLog) Take(to *BlockRef) *ReplacementStep {
	// Hand out the interval unless it was forfeited, then start the next.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	var out *ReplacementStep
	if !l.forfeit {
		step := l.step
		step.To = to.Clone()
		out = &step
	}
	l.reset(to)
	return out
}

// reset starts an empty interval at the root from. Assumes mtx is locked.
func (l *ReplacementLog) reset(from *BlockRef) {
	l.step = ReplacementStep{
		From:     from.Clone(),
		replaced: make(map[string]pageBase),
		created:  make(map[string]pageBase),
		original: make(map[string]*BlockRef),
	}
	l.forfeit = false
}

// RecordReplacements makes the transaction's writes record the stored pages
// they replace into log. Positions loaded before the call are recorded as if
// written without a base, so call it before reading.
func (t *Transaction) RecordReplacements(log *ReplacementLog) {
	if t == nil {
		return
	}
	t.mtx.Lock()
	t.replacements = log
	t.mtx.Unlock()
}

// GetReplacementLog returns the log the transaction records into, or nil.
func (t *Transaction) GetReplacementLog() *ReplacementLog {
	if t == nil {
		return nil
	}
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.replacements
}

// recordWrite records a page a write encoded: base is the stored page the
// position held, or nil, and page is what the write stored, or nil for an
// empty block.
func (l *ReplacementLog) recordWrite(base, page *pageBase) {
	// Re-encoding a page to the same ref changes nothing.
	if base != nil && page != nil && base.ref.EqualsRef(page.ref) {
		return
	}

	// Record the replaced base and the stored page. A base no earlier write
	// created is a page of the tree the interval began at.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if base != nil {
		key := base.ref.MarshalString()
		l.step.replaced[key] = *base
		if _, ok := l.step.created[key]; !ok {
			l.step.original[key] = base.ref
		}
	}
	if page != nil {
		l.step.created[page.ref.MarshalString()] = *page
	}
}

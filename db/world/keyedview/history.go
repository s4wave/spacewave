package keyedview

import (
	"container/list"
	"sync"
)

// History remembers the most recent Views by version, so a reader naming one
// can be answered with changes. Views are named by content, so readers in the
// same state share one entry. It forgets the least recently used View past
// its size and everything when the process ends; readers then read in full.
type History struct {
	// size is the most Views held.
	size int
	// mtx guards order and views.
	mtx sync.Mutex
	// order holds the Views, most recently used first.
	order list.List
	// views indexes order's elements by version.
	views map[string]*list.Element
}

// NewHistory builds a History holding at most size Views.
func NewHistory(size int) *History {
	return &History{size: size, views: make(map[string]*list.Element)}
}

// Add remembers view.
func (h *History) Add(view *View) {
	// Keep the latest use at the front and evict excess history.
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if elem, ok := h.views[view.Version]; ok {
		h.order.MoveToFront(elem)
		return
	}
	h.views[view.Version] = h.order.PushFront(view)
	if h.order.Len() > h.size {
		oldest := h.order.Back()
		h.order.Remove(oldest)
		delete(h.views, oldest.Value.(*View).Version)
	}
}

// Since returns what a reader holding version lacks to reach view, which the
// History then remembers.
func (h *History) Since(version string, view *View) Changes {
	// Remember the current view and compare the requested base when known.
	h.Add(view)
	if version == view.Version {
		return Changes{}
	}
	h.mtx.Lock()
	var earlier *View
	if elem, ok := h.views[version]; ok && version != "" {
		h.order.MoveToFront(elem)
		earlier = elem.Value.(*View)
	}
	h.mtx.Unlock()
	return view.Since(earlier)
}

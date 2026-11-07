package block_store_nodepush

import (
	"sync"
	"sync/atomic"
	"time"
)

// Held is the memory set of a local store whose pushed blocks skip its disk:
// each block's stored value, as the node holds it, from its publication until
// the node reports that the remote holds it, or until it spills to the local
// disk. Every read path of the store reads it first.
//
// Invariant: a block leaves Held only once the local pack index finds it on
// the remote (Drop, after the report's index entry is durable), or once the
// local store holds its value and queue entry durably (Drop, after a spill's
// flush), or when its publication failed or took the local path (Remove).
// Dropping it earlier loses the only copy besides the node's disk.
type Held struct {
	// limit bounds the bytes held; a publication that would pass it takes
	// the local path. Its owner may change it at any time: lowering it below
	// what is held drops nothing, and new publications take the local path
	// until reports and spills bring it under.
	limit *atomic.Int64

	// mtx guards the fields below.
	mtx sync.Mutex
	// blocks maps a pack key to its held block.
	blocks map[string]*heldBlock
	// bytes is the size of every held value.
	bytes int64
}

// heldBlock is one held block.
type heldBlock struct {
	// value is the stored value: the block's data and refs.
	value []byte
	// since is when it was first held.
	since time.Time
	// count is the publications holding it.
	count int
}

// NewHeld builds a Held of at most limit bytes.
func NewHeld(limit *atomic.Int64) *Held {
	return &Held{limit: limit, blocks: make(map[string]*heldBlock)}
}

// Add holds values by key, all or none, and reports false when they would
// pass the limit. A key already held is held once more.
func (h *Held) Add(keys, values [][]byte) bool {
	// Size what is new.
	h.mtx.Lock()
	defer h.mtx.Unlock()
	var size int64
	for i, key := range keys {
		if h.blocks[string(key)] == nil {
			size += int64(len(values[i]))
		}
	}
	if h.bytes+size > h.limit.Load() {
		return false
	}

	// Hold them.
	now := time.Now()
	for i, key := range keys {
		b := h.blocks[string(key)]
		if b == nil {
			b = &heldBlock{value: values[i], since: now}
			h.blocks[string(key)] = b
			h.bytes += int64(len(values[i]))
		}
		b.count++
	}
	return true
}

// Remove releases one publication's hold of keys, forgetting a block no
// publication holds any more.
func (h *Held) Remove(keys [][]byte) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	for _, key := range keys {
		b := h.blocks[string(key)]
		if b == nil {
			continue
		}
		if b.count--; b.count <= 0 {
			h.forgetLocked(string(key), b)
		}
	}
}

// Drop forgets keys whatever holds them: the local store can find each
// elsewhere now.
func (h *Held) Drop(keys [][]byte) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	for _, key := range keys {
		if b := h.blocks[string(key)]; b != nil {
			h.forgetLocked(string(key), b)
		}
	}
}

// forgetLocked forgets one block. The caller holds mtx.
func (h *Held) forgetLocked(key string, b *heldBlock) {
	delete(h.blocks, key)
	h.bytes -= int64(len(b.value))
}

// Get returns the stored value of key.
func (h *Held) Get(key []byte) ([]byte, bool) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if b := h.blocks[string(key)]; b != nil {
		return b.value, true
	}
	return nil, false
}

// Bytes returns the size of the held values.
func (h *Held) Bytes() int64 {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.bytes
}

// HeldSince returns up to limit keys held since before at, with their values,
// for a spill.
func (h *Held) HeldSince(at time.Time, limit int) (keys, values [][]byte) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	for key, b := range h.blocks {
		if len(keys) == limit {
			break
		}
		if b.since.Before(at) {
			keys = append(keys, []byte(key))
			values = append(values, b.value)
		}
	}
	return keys, values
}

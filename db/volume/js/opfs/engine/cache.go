package engine

import "context"

// cachedMessage reuses immutable decoded metadata without repeating protobuf work.
func (e *Engine) cachedMessage(ctx context.Context, name string) (message, error) {
	// Reject cancelled lookups before taking the engine lock.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Return the cached message and mark it recently used.
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if e.closed {
		return nil, ErrClosed
	}
	if elem := e.cache[name]; elem != nil {
		e.recency.MoveToFront(elem)
		return elem.Value.(*cacheEntry).parsed, nil
	}
	return nil, nil
}

// cacheMessage charges decoded metadata to the same byte and entry budgets.
// The conservative charge covers generated record objects and copied fields.
func (e *Engine) cacheMessage(name string, parsed message, decodedCharge int) {
	// Reject updates on a closed engine or an uncached name.
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if e.closed {
		return
	}
	elem := e.cache[name]
	if elem == nil {
		return
	}
	entry := elem.Value.(*cacheEntry)
	if entry.parsed != nil || entry.charge+decodedCharge > cacheByteLimit {
		return
	}

	// Evict least recently used entries until the charge fits the budget.
	e.recency.MoveToFront(elem)
	for e.cacheBytes+decodedCharge > cacheByteLimit {
		victim := e.recency.Back()
		old := victim.Value.(*cacheEntry)
		delete(e.cache, old.name)
		e.cacheBytes -= old.charge
		e.recency.Remove(victim)
	}

	// Store the parsed message and its charge on the retained entry.
	entry.parsed = parsed
	entry.charge += decodedCharge
	e.cacheBytes += decodedCharge
}

// cacheBytesLocked retains immutable bytes under the shared byte and entry budgets.
// The caller holds mtx and has checked that the engine remains open.
func (e *Engine) cacheBytesLocked(key string, data []byte) []byte {
	// Return cached bytes and mark the entry recently used.
	if elem := e.cache[key]; elem != nil {
		e.recency.MoveToFront(elem)
		return elem.Value.(*cacheEntry).data
	}

	// Evict least recently used entries until the new entry fits both budgets.
	charge := len(data) + len(key) + 192
	for e.cacheBytes+charge > cacheByteLimit || len(e.cache) >= cacheFileLimit {
		elem := e.recency.Back()
		entry := elem.Value.(*cacheEntry)
		delete(e.cache, entry.name)
		e.cacheBytes -= entry.charge
		e.recency.Remove(elem)
	}

	// Insert the new entry at the front of the recency list.
	e.cache[key] = e.recency.PushFront(&cacheEntry{name: key, data: data, charge: charge})
	e.cacheBytes += charge
	return data
}

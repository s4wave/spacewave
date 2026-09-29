package engine

import "context"

// protect shares one backend reclamation lease across active local operations.
// Nested work may join an existing lease even when exclusive reclamation waits.
func (e *Engine) protect(ctx context.Context) (func(), error) {
	// Serialize first acquisition without holding the cache/lifecycle mutex.
	unlock, err := e.pin.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	e.mtx.Lock()
	if e.closed {
		e.mtx.Unlock()
		return nil, ErrClosed
	}
	if e.readers != 0 {
		e.readers++
		e.mtx.Unlock()
		return e.unprotect, nil
	}
	e.mtx.Unlock()

	// Obtain cross-runtime protection before exposing the first local reader.
	release, err := e.backend.Lock(ctx, "reclaim", false)
	if err != nil {
		return nil, err
	}
	e.mtx.Lock()
	if e.closed {
		e.mtx.Unlock()
		release()
		return nil, ErrClosed
	}
	e.readers = 1
	e.releaseReaders = release
	e.mtx.Unlock()
	return e.unprotect, nil
}

// unprotect releases backend protection after the last local operation ends.
func (e *Engine) unprotect() {
	// Drop the local reader count and capture the backend release when empty.
	e.mtx.Lock()
	e.readers--
	var release func()
	if e.readers == 0 {
		release = e.releaseReaders
		e.releaseReaders = nil
	}
	e.mtx.Unlock()

	// Release backend protection once the last local reader leaves.
	if release != nil {
		release()
	}
}

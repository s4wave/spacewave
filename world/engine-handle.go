package world

import "context"

// EngineHandle is an open handle to an Engine.
// Typically attached to a client or using LookupBlockWorldEngine directive.
type EngineHandle interface {
	// GetContext returns the context of the engine handle.
	// Canceled when the handle is released.
	GetContext() context.Context
	// Engine implements the transactional engine interface.
	Engine
	// Release releases the engine handle.
	Release()
}

// engineHandle implements the EngineHandle interface.
type engineHandle struct {
	Engine
	ctx context.Context
	cc  context.CancelFunc
	rel func()
}

// NewEngineHandle constructs a new engine handle with a context.
// rel is an optional release callback.
func NewEngineHandle(ctx context.Context, e Engine, rel func()) EngineHandle {
	eg := &engineHandle{Engine: e, rel: rel}
	eg.ctx, eg.cc = context.WithCancel(ctx)
	return eg
}

// GetContext returns the context of the engine handle.
// Canceled when the handle is released.
func (e *engineHandle) GetContext() context.Context {
	return e.ctx
}

// Release releases the engine handle.
func (e *engineHandle) Release() {
	e.cc()
	if e.rel != nil {
		e.rel()
	}
}

// _ is a type assertion
var _ EngineHandle = ((*engineHandle)(nil))

package sobject

import "context"

// SOStateLock is a lock handle to a SOState.
type SOStateLock interface {
	// GetSOState returns the SOState as of when the lock was acquired.
	GetSOState() *SOState

	// WriteSOState writes an updated SOState.
	// Returns an error if the lock was released already.
	// Only the locked handle should be able to write.
	WriteSOState(ctx context.Context, state *SOState) error

	// Release releases the lock.
	Release()
}

// soStateLock implements SOStateLock.
type soStateLock struct {
	initialState *SOState
	writeFn      func(ctx context.Context, state *SOState) error
	release      func()
}

// GetSOState returns the SOState as of when the lock was acquired.
func (l *soStateLock) GetSOState() *SOState {
	return l.initialState
}

// WriteSOState writes an updated SOState.
// Returns an error if the lock was released already.
// Only the locked handle should be able to write.
func (l *soStateLock) WriteSOState(ctx context.Context, state *SOState) error {
	return l.writeFn(ctx, state)
}

// Release releases the lock.
func (l *soStateLock) Release() {
	if l.release != nil {
		l.release()
	}
}

// NewSOStateLock constructs a SOStateLock with an initial value and callbacks.
func NewSOStateLock(
	initialState *SOState,
	writeFn func(ctx context.Context, state *SOState) error,
	release func(),
) SOStateLock {
	return &soStateLock{initialState: initialState, writeFn: writeFn, release: release}
}

// _ is a type assertion
var _ SOStateLock = (*soStateLock)(nil)

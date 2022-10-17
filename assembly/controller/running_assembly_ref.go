package assembly_controller

import (
	"sync"

	"github.com/aperturerobotics/bldr/assembly"
)

// runningAssemblyRef implements the reference directive.
type runningAssemblyRef struct {
	mtx sync.Mutex
	cbs []func(assembly.State)
	rc  *runningAssembly // cannot be nil

	relOnce sync.Once
}

func newRunningAssemblyRef(rc *runningAssembly) *runningAssemblyRef {
	return &runningAssemblyRef{rc: rc}
}

// GetState returns the current state object.
func (r *runningAssemblyRef) GetState() assembly.State {
	r.mtx.Lock()
	rc := r.rc
	r.mtx.Unlock()
	if rc == nil {
		return &runningAssemblyState{}
	}

	rc.mtx.Lock()
	st := rc.state
	rc.mtx.Unlock()
	return &st
}

// AddStateCb adds a callback that is called when the state changes.
// Should not block.
// Will be called with the initial state.
func (r *runningAssemblyRef) AddStateCb(cb func(assembly.State)) {
	r.mtx.Lock()
	r.cbs = append(r.cbs, cb)
	rc := r.rc
	r.mtx.Unlock()
	cb(rc.GetState())
}

// GetRunningAssembly gets the running Assembly.
func (r *runningAssemblyRef) GetRunningAssembly() *runningAssembly {
	r.mtx.Lock()
	rc := r.rc
	r.mtx.Unlock()
	return rc
}

// pushState pushes an updated state
func (r *runningAssemblyRef) pushState(st assembly.State) {
	r.mtx.Lock()
	for _, cb := range r.cbs {
		cb(st)
	}
	r.mtx.Unlock()
}

// Release releases the reference.
func (r *runningAssemblyRef) Release() {
	r.relOnce.Do(func() {
		r.rc.Release()
	})
}

// _ is a type assertion
var _ assembly.Reference = ((*runningAssemblyRef)(nil))

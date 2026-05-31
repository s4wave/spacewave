package status

import (
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
)

type observedDirectiveKind int

const (
	observedDirectiveKindManifestFetch observedDirectiveKind = iota
	observedDirectiveKindController
)

type observedDirective struct {
	key     string
	kind    observedDirectiveKind
	update  func(isIdle bool, errs []error, vals []directive.AttachedValue)
	dispose func()

	mtx            sync.Mutex
	released       bool
	weakRef        directive.Reference
	releaseState   func()
	releaseDispose func()
}

type observedDirectiveSpec struct {
	key     string
	kind    observedDirectiveKind
	di      directive.Instance
	update  func(isIdle bool, errs []error, vals []directive.AttachedValue)
	dispose func()
}

func (o *observedDirective) configure(spec observedDirectiveSpec) {
	o.mtx.Lock()
	o.kind = spec.kind
	o.update = spec.update
	o.dispose = spec.dispose
	o.mtx.Unlock()
}

func (o *observedDirective) attach(di directive.Instance) {
	weakRef := di.AddReference(nil, true)
	releaseState := di.AddStateCallback(o.update)
	releaseDispose := di.AddDisposeCallback(o.dispose)

	o.mtx.Lock()
	if o.released {
		o.mtx.Unlock()
		releaseDispose()
		releaseState()
		weakRef.Release()
		return
	}
	o.weakRef = weakRef
	o.releaseState = releaseState
	o.releaseDispose = releaseDispose
	o.mtx.Unlock()
}

func (o *observedDirective) release() {
	o.mtx.Lock()
	if o.released {
		o.mtx.Unlock()
		return
	}
	o.released = true
	releaseDispose := o.releaseDispose
	o.releaseDispose = nil
	releaseState := o.releaseState
	o.releaseState = nil
	weakRef := o.weakRef
	o.weakRef = nil
	o.mtx.Unlock()

	if releaseDispose != nil {
		releaseDispose()
	}
	if releaseState != nil {
		releaseState()
	}
	if weakRef != nil {
		weakRef.Release()
	}
}

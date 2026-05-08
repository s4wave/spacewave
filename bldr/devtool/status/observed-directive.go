package status

import "github.com/aperturerobotics/controllerbus/directive"

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

	weakRef        directive.Reference
	releaseState   func()
	releaseDispose func()
}

func (o *observedDirective) attach(di directive.Instance) {
	o.weakRef = di.AddReference(nil, true)
	o.releaseState = di.AddStateCallback(o.update)
	o.releaseDispose = di.AddDisposeCallback(o.dispose)
}

func (o *observedDirective) release() {
	if o.releaseDispose != nil {
		o.releaseDispose()
		o.releaseDispose = nil
	}
	if o.releaseState != nil {
		o.releaseState()
		o.releaseState = nil
	}
	if o.weakRef != nil {
		o.weakRef.Release()
		o.weakRef = nil
	}
}

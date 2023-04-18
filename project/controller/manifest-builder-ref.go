package bldr_project_controller

import (
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
)

// ManifestBuilderRef is a reference to a manifest builder.
type ManifestBuilderRef struct {
	ref     *keyed.KeyedRef[string, *manifestBuilderTracker]
	tracker *manifestBuilderTracker
}

// newManifestBuilderRef constructs a ManifestBuilderRef.
func newManifestBuilderRef(ref *keyed.KeyedRef[string, *manifestBuilderTracker], tracker *manifestBuilderTracker) *ManifestBuilderRef {
	return &ManifestBuilderRef{ref: ref, tracker: tracker}
}

// GetManifestBuilderConfig returns the manifest bundle config.
func (r *ManifestBuilderRef) GetManifestBuilderConfig() *ManifestBuilderConfig {
	return r.tracker.conf
}

// GetResultPromiseContainer returns the result promise container.
func (r *ManifestBuilderRef) GetResultPromiseContainer() *promise.PromiseContainer[*ManifestBuilderResult] {
	return r.tracker.resultPromiseCtr
}

// Release releases the reference.
func (r *ManifestBuilderRef) Release() {
	r.ref.Release()
}

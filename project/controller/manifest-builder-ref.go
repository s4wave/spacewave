package bldr_project_controller

import (
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
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

// GetManifestMeta returns the manifest metadata.
func (r *ManifestBuilderRef) GetManifestMeta() *bldr_manifest.ManifestMeta {
	return r.tracker.meta
}

// GetResultPromise returns the result promise.
func (r *ManifestBuilderRef) GetResultPromise() promise.PromiseLike[*manifest_builder.BuilderResult] {
	return r.tracker.resultPromise
}

// Release releases the reference.
func (r *ManifestBuilderRef) Release() {
	r.ref.Release()
}

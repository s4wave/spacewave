package bldr_project_controller

import (
	plugin_builder "github.com/aperturerobotics/bldr/plugin/builder"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
)

// PluginBuilderRef is a reference to a plugin builder.
type PluginBuilderRef struct {
	ref     *keyed.KeyedRef[string, *pluginBuilderTracker]
	tracker *pluginBuilderTracker
}

// newPluginBuilderRef constructs a PluginBuilderRef.
func newPluginBuilderRef(ref *keyed.KeyedRef[string, *pluginBuilderTracker], tracker *pluginBuilderTracker) *PluginBuilderRef {
	return &PluginBuilderRef{ref: ref, tracker: tracker}
}

// GetResultPromise returns the result promise.
func (r *PluginBuilderRef) GetResultPromise() promise.PromiseLike[*plugin_builder.PluginBuilderResult] {
	return r.tracker.resultPromise
}

// Release releases the reference.
func (r *PluginBuilderRef) Release() {
	r.ref.Release()
}

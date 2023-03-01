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

// GetBuilderCtrlPromise returns the promise for the builder controller.
func (r *PluginBuilderRef) GetBuilderCtrlPromise() promise.PromiseLike[plugin_builder.Controller] {
	return r.tracker.builderCtrlPromise
}

// Release releases the reference.
func (r *PluginBuilderRef) Release() {
	r.ref.Release()
}

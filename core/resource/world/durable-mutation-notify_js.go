//go:build js

package resource_world

import "syscall/js"

// notifyDurableMutationToBrowser signals the browser document that a
// user-authored world mutation just fenced durable, so the document can request
// browser eviction protection on the first such write. It is a no-op when the
// browser global is absent.
func notifyDurableMutationToBrowser() {
	notify := js.Global().Get("BLDR_NOTIFY_DURABLE_MUTATION")
	if notify.IsUndefined() || notify.IsNull() || notify.Type() != js.TypeFunction {
		return
	}
	notify.Invoke()
}

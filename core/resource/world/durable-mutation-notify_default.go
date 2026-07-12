//go:build !js

package resource_world

// notifyDurableMutationToBrowser is a no-op outside the browser runtime.
func notifyDurableMutationToBrowser() {
}

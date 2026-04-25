//go:build js

package provider_spacewave

// buildSessionDeviceInfo returns a lightweight client-side platform hint for
// browser/WASM session registration metadata.
func buildSessionDeviceInfo() string {
	return "web"
}

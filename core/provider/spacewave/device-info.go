//go:build !js

package provider_spacewave

import "runtime"

// buildSessionDeviceInfo returns a lightweight client-side platform hint for
// session registration metadata.
func buildSessionDeviceInfo() string {
	return runtime.GOOS
}

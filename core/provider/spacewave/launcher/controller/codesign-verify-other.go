//go:build !darwin && !js

package spacewave_launcher_controller

import "context"

// verifyAppBundleCodesign is a no-op on non-darwin platforms. The .app bundle
// staging path only runs on macOS (see detectAppBundle); this stub keeps the
// package compiling for the other updater paths.
func verifyAppBundleCodesign(_ context.Context, _ string) error {
	return nil
}

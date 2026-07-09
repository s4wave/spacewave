//go:build windows || js

package spacewave_launcher_controller

// applyXattrsFromPAX is a no-op on platforms without xattr support.
func applyXattrsFromPAX(path string, pax map[string]string) {}

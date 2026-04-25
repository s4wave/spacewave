//go:build js

package spacewave_launcher_controller

// initUpdaterRoutine is a no-op on WASM (auto-update is desktop-only).
func (c *Controller) initUpdaterRoutine() {}

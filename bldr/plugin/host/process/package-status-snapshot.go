//go:build !js

package plugin_host_process

// PluginPackageStatusSnapshot is the read-only native package recovery status
// snapshot.
type PluginPackageStatusSnapshot struct {
	// Packages holds one status per plugin with a recorded action.
	Packages []PluginPackageStatus
}

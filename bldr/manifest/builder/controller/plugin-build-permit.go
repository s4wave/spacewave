//go:build !js

package bldr_manifest_builder_controller

// PluginBuildPermit represents one acquired plugin build slot.
type PluginBuildPermit struct {
	limiter *PluginBuildLimiter
}

// Release returns the plugin build slot.
func (p PluginBuildPermit) Release() {
	if p.limiter != nil {
		p.limiter.semaphore.Release(1)
	}
}

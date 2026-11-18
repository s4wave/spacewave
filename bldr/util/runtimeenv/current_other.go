//go:build !js

package runtimeenv

// Current retains baseline performance policy on native hosts.
func Current() Environment {
	return Environment{Engine: EngineUnknown}
}

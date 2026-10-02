//go:build !unix && !js

package spacewave_cli

// startKeyListener does not listen for keys on this platform.
func startKeyListener(func(byte)) (stop func(), listening bool) {
	return func() {}, false
}

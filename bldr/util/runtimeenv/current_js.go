//go:build js

package runtimeenv

import "syscall/js"

// current is detected once in each JavaScript host, including browser workers.
var current = detectCurrent()

// Current returns the host's immutable environment without shared mutable flags.
func Current() Environment {
	return current
}

// detectCurrent tolerates non-browser JavaScript hosts without a navigator.
func detectCurrent() Environment {
	navigator := js.Global().Get("navigator")
	if navigator.IsUndefined() || navigator.IsNull() {
		return Environment{Engine: EngineUnknown}
	}
	userAgent := navigator.Get("userAgent")
	if userAgent.Type() != js.TypeString {
		return Environment{Engine: EngineUnknown}
	}
	return Environment{Engine: detectEngine(userAgent.String())}
}

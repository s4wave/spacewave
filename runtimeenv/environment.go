// Package runtimeenv resolves process-local performance policy from runtime facts.
// Gates choose equivalent implementations; they do not grant capabilities or
// change stored formats. Unknown environments retain the baseline policy.
package runtimeenv

import "strings"

// Engine identifies the executing browser engine, independent of browser brand.
type Engine string

const (
	// EngineUnknown includes native hosts and unrecognized JavaScript hosts.
	EngineUnknown Engine = "unknown"
	// EngineChromium identifies Blink-based Chromium hosts, including Electron.
	EngineChromium Engine = "chromium"
	// EngineWebKit identifies Safari and other WebKit hosts.
	EngineWebKit Engine = "webkit"
)

// Environment contains immutable runtime facts used to resolve feature policy.
type Environment struct {
	// Engine identifies the host's browser engine.
	Engine Engine
}

// Feature names a performance choice with a baseline implementation.
type Feature uint8

const (
	// BatchCopyExistence resolves destination existence with each copy write batch.
	BatchCopyExistence Feature = iota + 1
)

// Enabled resolves a feature for this environment. Unknown features stay disabled.
func (e Environment) Enabled(feature Feature) bool {
	switch feature {
	case BatchCopyExistence:
		return e.Engine == EngineChromium
	default:
		return false
	}
}

// detectEngine recognizes engine tokens conservatively. iOS browser branding
// alone must not enable Chromium performance policy.
func detectEngine(userAgent string) Engine {
	// Classify iOS browser tokens before recognizing desktop Chromium engines.
	ua := strings.ToLower(userAgent)
	if strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "ipod") {
		if strings.Contains(ua, "applewebkit/") {
			return EngineWebKit
		}
		return EngineUnknown
	}

	// Recognize desktop engine tokens and preserve unknown hosts.
	if strings.Contains(ua, "chrome/") || strings.Contains(ua, "chromium/") {
		return EngineChromium
	}
	if strings.Contains(ua, "applewebkit/") {
		return EngineWebKit
	}
	return EngineUnknown
}

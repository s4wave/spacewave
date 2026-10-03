package runtimeenv

import "testing"

// TestEnginePolicy prevents browser brands and unknown hosts from enabling the
// Chromium optimization on WebKit or unsupported environments.
func TestEnginePolicy(t *testing.T) {
	// Cover browser engine tokens and hosts that must retain baseline policy.
	for _, test := range []struct {
		// name identifies the host variant.
		name string
		// ua is the host user agent.
		ua string
		// engine is the expected conservative engine classification.
		engine Engine
	}{
		{"chromium", "Mozilla/5.0 AppleWebKit/537.36 Chrome/140.0.0.0 Safari/537.36", EngineChromium},
		{"edge", "Mozilla/5.0 AppleWebKit/537.36 Chrome/140.0.0.0 Safari/537.36 Edg/140.0", EngineChromium},
		{"electron", "Mozilla/5.0 AppleWebKit/537.36 Chrome/140.0.0.0 Electron/40.0.0 Safari/537.36", EngineChromium},
		{"safari", "Mozilla/5.0 (Macintosh) AppleWebKit/605.1.15 Version/26.0 Safari/605.1.15", EngineWebKit},
		{"ios-chrome", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0) AppleWebKit/605.1.15 CriOS/140.0 Mobile Safari/604.1", EngineWebKit},
		{"ios-firefox", "Mozilla/5.0 (iPad; CPU OS 18_0) AppleWebKit/605.1.15 FxiOS/140.0 Mobile Safari/605.1.15", EngineWebKit},
		{"firefox", "Mozilla/5.0 Gecko/20100101 Firefox/140.0", EngineUnknown},
		{"node", "Node.js/24", EngineUnknown},
		{"empty", "", EngineUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Verify the host user agent resolves to its conservative engine class.
			engine := detectEngine(test.ua)
			if engine != test.engine {
				t.Fatalf("engine = %q, want %q", engine, test.engine)
			}

			// Verify Chromium alone enables batch existence checks and unknown features stay off.
			environment := Environment{Engine: engine}
			if got, want := environment.Enabled(BatchCopyExistence), test.engine == EngineChromium; got != want {
				t.Fatalf("batch copy existence = %t, want %t", got, want)
			}
			if environment.Enabled(Feature(255)) {
				t.Fatal("unknown feature enabled")
			}
		})
	}
}

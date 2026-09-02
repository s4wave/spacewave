package entrypoint_browser_bundle

import (
	"testing"

	"github.com/aperturerobotics/fastjson"
)

func TestBrowserRendererSpecDefinesTrustedIceServers(t *testing.T) {
	spec, err := browserRendererSpec(
		"/src", "/src/bldr", "/build", "", "", "", "", "", "",
		false, false, false, false, false,
		[]BrowserIceServer{{
			URLs:       []string{"stun:trusted.example:3478"},
			Username:   "user",
			Credential: "secret",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	const want = `[{"urls":["stun:trusted.example:3478"],"username":"user","credential":"secret"}]`
	if got := spec.Defines["BLDR_BROWSER_ICE_SERVERS"]; got != want {
		t.Fatalf("trusted ICE define = %q, want %q", got, want)
	}
}

func TestBrowserRendererSpecDefaultsAndEncodesIceServers(t *testing.T) {
	spec, err := browserRendererSpec(
		"/src", "/src/bldr", "/build", "", "", "", "", "", "",
		false, false, false, false, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	const wantDefault = `[{"urls":["stun:stun.l.google.com:19302"]}]`
	if got := spec.Defines["BLDR_BROWSER_ICE_SERVERS"]; got != wantDefault {
		t.Fatalf("default trusted ICE define = %q, want %q", got, wantDefault)
	}

	encoded := encodeBrowserIceServers([]BrowserIceServer{{
		URLs:       []string{"stun:example.test/\u0001"},
		Username:   "user\u0001",
		Credential: "secret\u0001",
	}})
	if err := fastjson.Validate(encoded); err != nil {
		t.Fatalf("trusted ICE JSON is invalid: %v: %q", err, encoded)
	}
}

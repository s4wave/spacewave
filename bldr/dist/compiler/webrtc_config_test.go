package bldr_dist_compiler

import "testing"

func TestBrowserIceServersForBundle(t *testing.T) {
	configured := browserIceServersForBundle([]*IceServer{{
		Urls:       []string{"turn:trusted.example:3478"},
		Username:   "user",
		Credential: "secret",
	}})
	if degenerate := browserIceServersForBundle([]*IceServer{nil, &IceServer{}}); len(degenerate) != 0 {
		t.Fatalf("degenerate ICE servers = %#v, want empty for bundle default", degenerate)
	}
	if len(configured) != 1 || configured[0].URLs[0] != "turn:trusted.example:3478" ||
		configured[0].Username != "user" || configured[0].Credential != "secret" {
		t.Fatalf("configured ICE servers = %#v", configured)
	}
}

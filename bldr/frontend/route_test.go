package bldr_frontend

import "testing"

// TestRouteService keeps each attachment's modules on its originating service.
func TestRouteService(t *testing.T) {
	for _, service := range []string{
		"devtool/" + SRPCFrontendServiceID,
		"plugin/spacewave-core/frontend/author/" + SRPCFrontendServiceID,
	} {
		got, err := RouteService(ServiceRoutePrefix(service) + "session/Viewer.tsx")
		if err != nil || got != service {
			t.Fatalf("routed %q to %q: %v", service, got, err)
		}
	}
	got, err := RouteService("/b/fe/session/Viewer.tsx")
	if err != nil || got != "devtool/"+SRPCFrontendServiceID {
		t.Fatalf("ordinary development route changed: %q %v", got, err)
	}
	for _, invalid := range []string{
		"/b/fe/rpc/invalid/session/module.js",
		ServiceRoutePrefix("plugin/spacewave-core/private.Service") + "session/module.js",
		ServiceRoutePrefix("devtool/" + SRPCFrontendServiceID),
	} {
		if _, err := RouteService(invalid); err == nil {
			t.Fatalf("accepted invalid module route %q", invalid)
		}
	}
}

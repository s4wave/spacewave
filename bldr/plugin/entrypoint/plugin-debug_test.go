//go:build !js

package plugin_entrypoint

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/sirupsen/logrus"
)

// TestServePluginDebugNamesProcessAndServesStacks reads a running plugin's
// identity and goroutine stacks through its debug socket.
func TestServePluginDebugNamesProcessAndServesStacks(t *testing.T) {
	// Serve the debug socket in a short private directory.
	root, err := os.MkdirTemp("", "dbg")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	info := bldr_plugin.NewPluginStartInfo("inst1", "glados-core", "space-abc", "manifestroot")
	servePluginDebug(ctx, logrus.NewEntry(logrus.New()), info, root)

	// Read the endpoints the way an operator would, over the socket.
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", info.DebugSocketPath(root))
		},
	}}
	get := func(path string) string {
		// Fetch the endpoint and require a 200 response.
		resp, err := client.Get("http://plugin" + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status %d: %v", path, resp.StatusCode, err)
		}
		return string(body)
	}

	// The identity identifies the plugin and its instance.
	identity := get("/debug/plugin")
	for _, want := range []string{"plugin-id=glados-core\n", "instance-key=space-abc\n", "instance-id=inst1\n", "pid=" + strconv.Itoa(os.Getpid()) + "\n"} {
		if !strings.Contains(identity, want) {
			t.Fatalf("identity %q lacks %q", identity, want)
		}
	}

	// Full goroutine stacks include this test's own goroutine.
	if stacks := get("/debug/pprof/goroutine?debug=2"); !strings.Contains(stacks, "TestServePluginDebugNamesProcessAndServesStacks") {
		t.Fatalf("goroutine dump lacks the test goroutine:\n%s", stacks)
	}
}

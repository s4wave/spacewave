//go:build !js

package resource_root

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/util"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/bldr/testbed"
	"github.com/sirupsen/logrus"
)

// TestBoundListenerRetainsManifest keeps an open listener's shell, modules and
// package chunks on its original release while a new listener sees the upgrade.
func TestBoundListenerRetainsManifest(t *testing.T) {
	// Build the real scheduler over an in-memory World.
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.BuildTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Publish browser manifests whose stable file names identify their release.
	publish := func(rev uint64, text string) {
		t.Helper()
		for _, pluginID := range BoundPluginIDs {
			files := memfs.New()
			filePath := "v/b/fe/entry.mjs"
			if pluginID == webPkgPluginID {
				filePath = bldr_plugin.PluginAssetsWebPkgsDir + "/sonner/chunk.mjs"
			}
			if err := util.WriteFile(files, filePath, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			meta := bldr_manifest.NewManifestMeta(pluginID, bldr_manifest.BuildType_DEV, "js", rev)
			_, ref, err := tb.CreateManifestWithBilly(ctx, meta, "entry.mjs", files, files, nil)
			if err != nil {
				t.Fatal(err)
			}

			// Retain each published artifact as the release catalog does.
			if err := bldr_manifest_world.ExStoreManifestOp(ctx, tb.GetWorldState(), tb.GetVolume().GetPeerID(), bldr_manifest.NewManifestArtifactKey(ref.GetManifestRef()), []string{tb.GetPluginHostObjKey()}, ref); err != nil {
				t.Fatal(err)
			}
		}
	}
	publish(1, "old release")

	// Open and authorize a listener while the first release is current.
	open := func() (*webListener, *http.Cookie) {
		// Open a listener on the current release and retain its lifetime.
		t.Helper()
		listener, err := newWebListener(ctx, le, tb.GetBus(), srpc.NewMux(), &webListenSpec{
			host: "127.0.0.1", spaceID: "bound", sessionIdx: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(listener.Close)

		// Exchange the bootstrap secret for the listener capability.
		secret, err := listener.issueBootstrapSecret()
		if err != nil {
			t.Fatal(err)
		}
		resp, err := exchangeWebBootstrapWithSecret(listener, secret)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return listener, findWebCapabilityCookie(resp.Cookies())
	}
	oldListener, oldCookie := open()
	if !strings.Contains(oldListener.appFrontendPath, "/manifest/") {
		t.Fatal("listener opened without a manifest binding")
	}

	// Announce a newer release and open a listener against that selection.
	publish(2, "new release")
	newListener, newCookie := open()
	if oldListener.appFrontendPath == newListener.appFrontendPath {
		t.Fatal("new listener did not select the announced release")
	}

	// Read both pinned app modules and redirected package chunks over HTTP.
	for _, test := range []struct {
		// listener is the release-bound HTTP server.
		listener *webListener
		// cookie authorizes the listener's asset requests.
		cookie *http.Cookie
		// want is the release's stable-file content.
		want string
	}{
		{oldListener, oldCookie, "old release"},
		{newListener, newCookie, "new release"},
	} {
		for _, assetPath := range []string{test.listener.appFrontendPath + "entry.mjs", "/b/pkg/sonner/chunk.mjs"} {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, test.listener.url+assetPath, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(test.cookie)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != http.StatusOK || string(data) != test.want {
				t.Fatalf("%s: status=%d body=%q err=%v", assetPath, resp.StatusCode, data, err)
			}
		}
	}

	// Render the original shell after the upgrade and retain its manifest URL.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<script type="importmap">{"imports":{}}</script>`)
	}))
	defer upstream.Close()
	t.Setenv("SPACEWAVE_WEB_ENDPOINT", upstream.URL)
	rec := httptest.NewRecorder()
	oldListener.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), oldListener.appFrontendPath) || strings.Contains(rec.Body.String(), newListener.appFrontendPath) {
		t.Fatal("open listener's shell changed manifest after the upgrade")
	}
}

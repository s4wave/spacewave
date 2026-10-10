package bldr_web_plugin_controller

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/go-git/go-billy/v6/memfs"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
	web_view_handler "github.com/s4wave/spacewave/bldr/web/view/handler"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// pinnedFiles serves one assets filesystem and counts the open handles.
type pinnedFiles struct {
	// fsID is the only filesystem served.
	fsID string
	// held counts the handles opened on fsID and not yet released.
	held atomic.Int32
	// released receives a signal when a handle on fsID is released.
	released chan struct{}
}

// GetControllerInfo returns information about the controller.
func (f *pinnedFiles) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/pinned-files", Version, "pinned files")
}

// Execute waits for the context to end.
func (f *pinnedFiles) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// HandleDirective resolves AccessUnixFS for fsID with a counting access func.
func (f *pinnedFiles) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	access, ok := inst.GetDirective().(unixfs_access.AccessUnixFS)
	if !ok || access.AccessUnixFSID() != f.fsID {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]unixfs_access.AccessUnixFSValue{
		func(context.Context, func()) (*unixfs.FSHandle, func(), error) {
			files, err := unixfs.NewFSHandle(unixfs_billy.NewBillyFSCursor(memfs.New(), ""))
			if err != nil {
				return nil, nil, err
			}
			f.held.Add(1)
			return files, func() {
				files.Release()
				f.held.Add(-1)
				f.released <- struct{}{}
			}, nil
		},
	}), nil)
}

// Close releases the controller.
func (f *pinnedFiles) Close() error {
	return nil
}

// TestBindHandlerAssetsHoldsPinnedFiles checks that the plugin files selected
// by handler URLs stay open until release, once per manifest, while unpinned
// URLs open nothing.
func TestBindHandlerAssetsHoldsPinnedFiles(t *testing.T) {
	// Run a real bus.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	// Serve one pinned assets filesystem on it.
	root, err := hash.Sum(hash.RecommendedHashType, []byte("release"))
	if err != nil {
		t.Fatal(err)
	}
	artifact := bldr_plugin.PluginArtifactID("spacewave-app", root.MarshalString())
	files := &pinnedFiles{fsID: bldr_plugin.PluginAssetsFsId(artifact), released: make(chan struct{}, 1)}
	releaseFiles, err := b.AddController(ctx, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFiles()

	// Publish the pinned entry, its package prefix, and its stylesheet, with
	// an unpinned package that must not be requested.
	prefix := bldr_plugin.PluginAssetsHttpPrefix + artifact
	handlers := &web_view_handler.WebViewHandlersConfig{Handlers: []*web_view_handler.WebViewHandlerConfig{
		{Handler: &web_view_handler.WebViewHandlerConfig_SetRenderMode{SetRenderMode: &web_view.SetRenderModeRequest{
			ScriptPath: prefix + "/v/b/fe/app/App.mjs?bldr_content=1",
			WebPkgPaths: map[string]string{
				"react": prefix + "/bldr-web-pkgs/react/",
				"other": bldr_plugin.PluginAssetsHttpPrefix + "spacewave-web/bldr-web-pkgs/other/",
			},
		}}},
		{Handler: &web_view_handler.WebViewHandlerConfig_SetHtmlLinks{SetHtmlLinks: &web_view.SetHtmlLinksRequest{
			SetLinks: map[string]*web_view.HtmlLink{"app.css": {Href: prefix + "/v/b/fe/app.css", Rel: "stylesheet"}},
		}}},
	}}

	// Bind the URLs and check one handle holds the pinned files.
	release, err := NewController(le, b, nil).bindHandlerAssets(ctx, handlers)
	if err != nil {
		t.Fatal(err)
	}
	if held := files.held.Load(); held != 1 {
		t.Fatalf("held %d pinned filesystems, want 1", held)
	}

	// Releasing the binding closes the handle.
	release()
	select {
	case <-files.released:
	case <-ctx.Done():
		t.Fatal("pinned files stayed open after release")
	}
}

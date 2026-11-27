//go:build !js

package plugin_entrypoint

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_pipesock "github.com/s4wave/spacewave/bldr/util/pipesock"
	"github.com/s4wave/spacewave/db/daemon/prof"
	"github.com/sirupsen/logrus"
)

// pluginIdentityText identifies one plugin process for a person reading its
// debug endpoint: which plugin, and which instance (for a Space, its
// installation).
// The instance key is empty for a shared plugin.
func pluginIdentityText(info *bldr_plugin.PluginStartInfo) string {
	return fmt.Sprintf(
		"pid=%d\nplugin-id=%s\ninstance-key=%s\ninstance-id=%s\nmanifest-root=%s\n",
		os.Getpid(), info.GetPluginId(), info.GetInstanceKey(), info.GetInstanceId(), info.GetManifestRoot(),
	)
}

// servePluginDebug serves pprof and the process identity on the plugin's debug
// socket until ctx ends. The socket lives in the host's private pipe directory,
// so only the host's user reaches it. A failure to serve is logged and does not
// stop the plugin: diagnostics must not take the plugin down.
func servePluginDebug(ctx context.Context, le *logrus.Entry, info *bldr_plugin.PluginStartInfo, pipeRoot string) {
	// Bind the debug socket beside the plugin pipe.
	path := info.DebugSocketPath(pipeRoot)
	lis, err := bldr_pipesock.ListenProtectedUnix(path)
	if err != nil {
		le.WithError(err).Warn("unable to serve plugin debug socket")
		return
	}

	// Serve pprof, plus an endpoint that identifies this process.
	identity := pluginIdentityText(info)
	mux := prof.NewProfMux()
	mux.HandleFunc("/debug/plugin", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, identity)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
		_ = os.Remove(filepath.Clean(path))
	}()
	go func() {
		if err := srv.Serve(lis); err != nil && err != http.ErrServerClosed {
			le.WithError(err).Warn("plugin debug socket exited")
		}
	}()
}

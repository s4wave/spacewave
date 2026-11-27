//go:build !js

package devtool

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	bldr_pipesock "github.com/s4wave/spacewave/bldr/util/pipesock"
	"github.com/s4wave/spacewave/db/daemon/prof"
	"github.com/sirupsen/logrus"
)

// DevtoolDebugSocketPath returns the Unix socket where the devtool serves
// pprof and its identity, inside a private directory of the state root.
func DevtoolDebugSocketPath(stateRoot string) string {
	return filepath.Join(stateRoot, "debug", "devtool.sock")
}

// serveDevtoolDebug serves pprof and the process identity on the devtool's
// debug socket until ctx ends, so a stuck build loop is traceable with
// curl --unix-socket <socket> http://devtool/debug/pprof/goroutine?debug=2.
// The caller holds the state lock, so a socket left by an earlier process is
// stale. A failure to serve is logged and does not stop the devtool.
func serveDevtoolDebug(ctx context.Context, le *logrus.Entry, stateRoot string) {
	// Replace any stale socket, then bind the debug socket.
	path := DevtoolDebugSocketPath(stateRoot)
	_ = os.Remove(path)
	lis, err := bldr_pipesock.ListenProtectedUnix(path)
	if err != nil {
		le.WithError(err).Warn("unable to serve devtool debug socket")
		return
	}

	// Serve pprof, plus an endpoint that identifies this process.
	identity := "pid=" + strconv.Itoa(os.Getpid()) + "\nstate-root=" + stateRoot + "\n"
	mux := prof.NewProfMux()
	mux.HandleFunc("/debug/devtool", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, identity)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
		_ = os.Remove(path)
	}()
	go func() {
		if err := srv.Serve(lis); err != nil && err != http.ErrServerClosed {
			le.WithError(err).Warn("devtool debug socket exited")
		}
	}()
	le.WithField("debug-socket", path).Info("serving devtool debug socket")
}

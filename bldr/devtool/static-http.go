//go:build !js

package devtool

import (
	"context"
	"net/http"
	"time"

	"github.com/s4wave/spacewave/net/http/assets"
)

// ExecuteStaticHttpServer runs the static http server command.
func (a *DevtoolArgs) ExecuteStaticHttpServer(ctx context.Context) error {
	// Resolve the static server address and filesystem root.
	le := a.logger
	listenAddr := a.WebListenAddr
	servePath := a.ServeStaticPath
	if servePath == "" {
		servePath = "./"
	}

	// run the http server
	serveFs := http.Dir(servePath)
	fileServer := assets.NewEncodedAssetFileServer(serveFs)

	// Wrap the handler with Cross-Origin Isolation headers required for
	// SharedArrayBuffer
	handler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		rw.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		fileServer.ServeHTTP(rw, req)
	})

	// Start the static HTTP server with the configured listener.
	le.Infof("listening on: %s", listenAddr)
	hserver := &http.Server{Addr: listenAddr, Handler: handler, ReadHeaderTimeout: time.Second * 30}
	return hserver.ListenAndServe()
}

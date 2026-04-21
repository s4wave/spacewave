//go:build !js

package devtool

import (
	"context"
	"net/http"
	"time"
)

// ExecuteStaticHttpServer runs the static http server command.
func (a *DevtoolArgs) ExecuteStaticHttpServer(ctx context.Context) error {
	le := a.Logger
	listenAddr := a.WebListenAddr
	servePath := a.ServeStaticPath
	if servePath == "" {
		servePath = "./"
	}

	// run the http server
	serveFs := http.Dir(servePath)
	fileServer := http.FileServer(serveFs)

	// Wrap with Cross-Origin Isolation headers required for SharedArrayBuffer
	handler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		rw.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		fileServer.ServeHTTP(rw, req)
	})

	le.Infof("listening on: %s", listenAddr)
	hserver := &http.Server{Addr: listenAddr, Handler: handler, ReadHeaderTimeout: time.Second * 30}
	return hserver.ListenAndServe()
}

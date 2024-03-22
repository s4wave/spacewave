package devtool

import (
	"context"
	"net/http"
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
	server := http.FileServer(serveFs)

	le.Infof("listening on: %s", listenAddr)
	return http.ListenAndServe(listenAddr, server)
}

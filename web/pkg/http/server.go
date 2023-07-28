package web_pkg_http

import (
	"context"
	"net/http"

	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	"github.com/aperturerobotics/controllerbus/bus"
	unixfs_http "github.com/aperturerobotics/hydra/unixfs/http"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Server serves web packages by performing LookupWebPkg directives.
type Server struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// returnIfIdle indicates we will return 404 if not found
	// if set to false, we will wait for the web pkg to be available.
	returnIfIdle bool
}

// NewServer constructs a new server.
func NewServer(le *logrus.Entry, b bus.Bus, returnIfIdle bool) *Server {
	return &Server{
		le:           le,
		b:            b,
		returnIfIdle: returnIfIdle,
	}
}

// ServeWebModuleHTTP serves an http request with the package path (e.x. react/index.js).
func (s *Server) ServeWebModuleHTTP(pkgPath string, rw http.ResponseWriter, req *http.Request) {
	s.le.
		WithField("pkg-path", pkgPath).
		Debug("forwarding pkg request")
	err := func(ctx context.Context) error {
		webPkgID, webPkgPath, err := web_pkg.CheckStripWebPkgIdPrefix(pkgPath)
		if err != nil {
			rw.WriteHeader(400)
			return err
		}

		webPkg, _, webPkgRef, err := web_pkg.ExLookupWebPkg(ctx, s.b, true, webPkgID)
		if err != nil {
			rw.WriteHeader(500)
			return err
		}
		if webPkgRef != nil {
			defer webPkgRef.Release()
		}

		if webPkg == nil {
			rw.WriteHeader(404)
			return errors.Errorf("web pkg not found: %v", webPkgID)
		}

		fsHandle, err := webPkg.GetWebPkgFsHandle(ctx)
		if err != nil {
			return err
		}
		defer fsHandle.Release()

		fs, err := unixfs_http.NewFileSystem(ctx, fsHandle, "")
		if err != nil {
			return err
		}

		req.URL.Path = webPkgPath
		handler := http.FileServer(fs)
		handler.ServeHTTP(rw, req)
		return nil
	}(req.Context())
	if err != nil && err != context.Canceled {
		s.le.
			WithError(err).
			WithField("pkg-path", pkgPath).
			Warn("pkg request failed")
		rw.WriteHeader(500) // only applies if we didn't call WriteHeader above.
		_, _ = rw.Write([]byte("web pkg " + pkgPath + ": " + err.Error()))
		return
	}
}

package web_pkg_http

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/controllerbus/bus"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	unixfs_http "github.com/s4wave/spacewave/db/unixfs/http"
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
	ctx := req.Context()
	webPkgID, webPkgPath, err := web_pkg.CheckStripWebPkgIdPrefix(pkgPath)
	if err != nil {
		http.Error(rw, "web pkg "+pkgPath+": "+err.Error(), http.StatusBadRequest)
		return
	}

	webPkg, _, webPkgRef, err := web_pkg.ExLookupWebPkg(ctx, s.b, s.returnIfIdle, webPkgID)
	if err != nil {
		if err != context.Canceled {
			s.le.WithError(err).WithField("pkg-path", pkgPath).Warn("pkg lookup failed")
			http.Error(rw, "web pkg "+pkgPath+": "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if webPkgRef != nil {
		defer webPkgRef.Release()
	}

	if webPkg == nil {
		http.Error(rw, "web pkg not found: "+webPkgID, http.StatusNotFound)
		return
	}

	fsHandle, err := webPkg.GetWebPkgFsHandle(ctx)
	if err != nil {
		if err != context.Canceled {
			s.le.WithError(err).WithField("pkg-path", pkgPath).Warn("pkg fs handle failed")
			http.Error(rw, "web pkg "+pkgPath+": "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	defer fsHandle.Release()

	fs, err := unixfs_http.NewFileSystem(ctx, fsHandle, "")
	if err != nil {
		if err != context.Canceled {
			s.le.WithError(err).WithField("pkg-path", pkgPath).Warn("pkg filesystem failed")
			http.Error(rw, "web pkg "+pkgPath+": "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	req.URL.Path = "/" + webPkgPath
	http.FileServer(fs).ServeHTTP(rw, req)
}

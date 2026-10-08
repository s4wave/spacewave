package web_pkg_http

import (
	"net/http"
	"net/url"
)

// RedirectToPinnedBase redirects the request to relPath under basePath, the
// location that pins a package's files to one release. The target is not
// cacheable, so a later release can pin a different base. The request's query
// is kept, and relPath is escaped as path data so a decoded "?" or "%" in a
// file name cannot change the target.
func RedirectToPinnedBase(rw http.ResponseWriter, req *http.Request, basePath, relPath string) {
	target, err := url.JoinPath(basePath, relPath)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	if req.URL.RawQuery != "" {
		target += "?" + req.URL.RawQuery
	}
	rw.Header().Set("Cache-Control", "no-store")
	http.Redirect(rw, req, target, http.StatusTemporaryRedirect)
}

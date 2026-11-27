package web_pkg_http

import (
	"net/http"
	"net/url"
	"strings"
)

// RedirectToPinnedBase redirects the request to relPath under basePath, the
// location that pins a package's files to one release. The target is not
// cacheable, so a later release can pin a different base. The request's query
// is kept, and relPath is escaped as path data so a decoded "?" or "%" in a
// file name cannot change the target. The target's scheme and host are
// basePath's, and a relPath whose ".." climbs out of basePath gets a not-found
// response, so the redirect stays inside the pinned base.
func RedirectToPinnedBase(rw http.ResponseWriter, req *http.Request, basePath, relPath string) {
	// Parse the base, which alone sets the target's scheme and host.
	base, err := url.Parse(basePath)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Refuse a file path whose ".." climbs out of the base: JoinPath cleans
	// the joined path, which resolves each "..".
	target := base.JoinPath(relPath)
	prefix := strings.TrimSuffix(base.EscapedPath(), "/") + "/"
	if !strings.HasPrefix(target.EscapedPath()+"/", prefix) {
		http.NotFound(rw, req)
		return
	}

	// Keep the request's query, and keep the redirect out of every cache.
	target.RawQuery = req.URL.RawQuery
	rw.Header().Set("Cache-Control", "no-store")

	// Redirect to the target. G710 traces the request's query into it, but
	// the program builds basePath, which sets the scheme and host, the prefix
	// check keeps the path inside it, and the query follows "?", where it
	// cannot change the origin.
	//nolint:gosec // G710: the target is confined to basePath, as above.
	http.Redirect(rw, req, target.String(), http.StatusTemporaryRedirect)
}

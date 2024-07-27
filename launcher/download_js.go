//go:build js

package bldr_launcher

import "github.com/aperturerobotics/bldr/util/http"

// setFetchDistConfigHttpOpts sets opts for fetch dist config.
func setFetchDistConfigHttpOpts(r *http.Request) {
	r.Cache = "reload"
}

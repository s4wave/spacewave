//go:build !tinygo

package nethttp

import (
	"io"
	"net/http"
)

// DrainAndCloseResponseBody drains unread response bytes and closes the body.
func DrainAndCloseResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

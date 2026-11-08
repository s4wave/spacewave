package webrtc_relay

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/s4wave/spacewave/net/peer"
)

// maxResponseSize bounds a credentials response read by the client.
const maxResponseSize = 64 * 1024

// ErrStatus is returned when the credentials route answers without credentials.
var ErrStatus = errors.New("relay credentials route refused the request")

// NewHandler serves issuer's credentials to the session auth names.
//
// auth returns the signed-in session's peer, or false to refuse the request.
// The response is the binary Credentials message and is never cached. Issuer
// errors answer 503 without their text, so an upstream reply or token cannot
// reach the client.
func NewHandler(issuer Issuer, auth func(r *http.Request) (peer.ID, bool)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Credentials are read, never written.
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Only a signed-in session draws on the account.
		session, ok := auth(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Issue for this session; the reason for a failure stays on the server.
		creds, err := issuer.Issue(r.Context(), session)
		if err != nil {
			http.Error(w, "relay unavailable", http.StatusServiceUnavailable)
			return
		}
		data, err := creds.MarshalVT()
		if err != nil {
			http.Error(w, "relay unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	})
}

// NewHTTPFetch fetches credentials from a route NewHandler serves.
//
// client carries the session's authentication, such as a cookie jar or a
// transport adding its token.
func NewHTTPFetch(client *http.Client, url string) Fetch {
	return func(ctx context.Context) (*Credentials, error) {
		// Ask the app server for this session's credentials.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, ErrStatus
		}

		// Read the bounded message.
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxResponseSize {
			return nil, ErrStatus
		}
		creds := &Credentials{}
		if err := creds.UnmarshalVT(data); err != nil {
			return nil, err
		}
		return creds, nil
	}
}

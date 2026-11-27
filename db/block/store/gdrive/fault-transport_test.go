//go:build !tinygo

package block_store_gdrive_test

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/s4wave/spacewave/db/block/store/gdrive/gdrivetest"
)

// faultTransport interrupts exactly one chosen resumable chunk.
type faultTransport struct {
	// base executes the fake HTTP request.
	base http.RoundTripper
	// server owns the upload sessions to expire.
	server *gdrivetest.Server
	// at is the one-based chunk to interrupt, then zero after firing.
	at int
	// chunks counts non-probe chunk requests.
	chunks int
	// expire forgets the session before a chunk; otherwise loses its reply.
	expire bool
	// loseFolder discards one committed folder-create reply to force a duplicate.
	loseFolder bool
	// status rejects the chosen chunk with a transient response and Retry-After.
	status int
	// rejectedAt records the transient answer's delivery to the client.
	rejectedAt time.Time
	// retryDelay records the next request's delay after that answer.
	retryDelay time.Duration
}

// RoundTrip expires a session or loses a reply after its chunk committed.
func (f *faultTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !f.rejectedAt.IsZero() && f.retryDelay == 0 {
		f.retryDelay = time.Since(f.rejectedAt)
	}
	// Count media chunks independently of the status probes after a lost reply.
	chunk := req.Method == http.MethodPut && !strings.HasPrefix(req.Header.Get("Content-Range"), "bytes */")
	if chunk {
		f.chunks++
	}
	folder := f.loseFolder && req.Method == http.MethodPost && req.URL.Path == "/drive/v3/files"
	fire := folder || chunk && f.at != 0 && f.chunks == f.at
	if fire && f.expire {
		f.server.ExpireUploads()
	}
	if fire && f.status != 0 {
		f.server.Inject(gdrivetest.Failure{Status: f.status})
	}
	resp, err := f.base.RoundTrip(req)
	if !fire || err != nil {
		return resp, err
	}

	// Keep the server's completed write while discarding only its reply.
	f.at = 0
	f.loseFolder = false
	if f.status != 0 {
		resp.Header.Set("Retry-After", "1")
		f.rejectedAt = time.Now()
		return resp, nil
	}
	if !f.expire {
		_ = resp.Body.Close() // Simulate a lost connection after committing a chunk.
		return nil, errors.New("injected lost upload reply")
	}
	return resp, nil
}

// _ is a type assertion.
var _ http.RoundTripper = (*faultTransport)(nil)

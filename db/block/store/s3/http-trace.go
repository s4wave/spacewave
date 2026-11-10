//go:build !tinygo

package block_store_s3

// REHEARSAL ONLY, never shipped: with SPACEWAVE_S3_TRACE_BUCKET and
// SPACEWAVE_S3_TRACE_PREFIX set, every S3 request is checked and logged to
// stderr. A request is refused unless it names that bucket and a key or list
// prefix under that prefix, and every DELETE is refused. Each request logs its
// method, key, Range and the response body bytes read.

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type traceTransport struct {
	next           http.RoundTripper
	bucket, prefix string
	n, bytes       atomic.Int64
}

type traceBody struct {
	io.ReadCloser
	t      *traceTransport
	method string
	key    string
	rng    string
	status int
	read   int64
}

func (b *traceBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.read += int64(n)
	return n, err
}

func (b *traceBody) Close() error {
	b.t.bytes.Add(b.read)
	fmt.Fprintf(os.Stderr, "%s S3TRACE %s %s range=%q status=%d bytes=%d total_req=%d total_bytes=%d\n",
		time.Now().UTC().Format("15:04:05.000Z"), b.method, b.key, b.rng, b.status, b.read, b.t.n.Load(), b.t.bytes.Load())
	return b.ReadCloser.Close()
}

// objectKey returns the key a request names, and the list prefix for a list.
func (t *traceTransport) objectKey(r *http.Request) (string, bool) {
	// Take the bucket off the host or the path.
	path := strings.TrimPrefix(r.URL.Path, "/")
	if strings.HasPrefix(r.URL.Host, t.bucket+".") {
		// Virtual-hosted style: the path is the key.
	} else if rest, ok := strings.CutPrefix(path, t.bucket); ok && (rest == "" || rest[0] == '/') {
		path = strings.TrimPrefix(rest, "/")
	} else {
		return "", false
	}

	// A list names its prefix in the query.
	if path == "" {
		pfx := r.URL.Query().Get("prefix")
		return "?prefix=" + pfx, strings.HasPrefix(pfx, t.prefix)
	}
	return path, strings.HasPrefix(path, t.prefix)
}

func (t *traceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// Refuse anything outside the fence, and every delete.
	key, ok := t.objectKey(r)
	if !ok || r.Method == http.MethodDelete {
		fmt.Fprintf(os.Stderr, "S3TRACE REFUSED %s host=%s path=%s\n", r.Method, r.URL.Host, r.URL.Path)
		return nil, fmt.Errorf("rehearsal trace: refused %s %s%s", r.Method, r.URL.Host, r.URL.Path)
	}

	// Send it, and count the body bytes the caller reads.
	t.n.Add(1)
	resp, err := t.next.RoundTrip(r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "S3TRACE %s %s err=%v\n", r.Method, key, err)
		return nil, err
	}
	resp.Body = &traceBody{ReadCloser: resp.Body, t: t, method: r.Method, key: key, rng: r.Header.Get("Range"), status: resp.StatusCode}
	return resp, nil
}

// traced wraps rt when the rehearsal trace is configured.
func traced(rt http.RoundTripper) http.RoundTripper {
	bucket, prefix := os.Getenv("SPACEWAVE_S3_TRACE_BUCKET"), os.Getenv("SPACEWAVE_S3_TRACE_PREFIX")
	if bucket == "" || prefix == "" {
		return rt
	}
	return &traceTransport{next: rt, bucket: bucket, prefix: prefix}
}

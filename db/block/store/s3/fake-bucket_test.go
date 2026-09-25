package block_store_s3

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeBucket is an in-memory S3 bucket named "bucket" serving PUT, ranged GET,
// DELETE, and ListObjectsV2 by prefix.
type fakeBucket struct {
	mtx     sync.Mutex
	objects map[string]string
	// puts counts the PUT requests.
	puts int
}

// newFakeBucket serves a bucket holding objects until the test ends.
func newFakeBucket(t *testing.T, objects map[string]string) (*fakeBucket, *Client) {
	t.Helper()
	b := &fakeBucket{objects: objects}
	if b.objects == nil {
		b.objects = make(map[string]string)
	}
	srv := httptest.NewServer(b)
	t.Cleanup(srv.Close)
	return b, newTestClient(t, srv)
}

// ServeHTTP answers one S3 request.
func (b *fakeBucket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	switch {
	case r.URL.Query().Get("list-type") == "2":
		prefix := r.URL.Query().Get("prefix")
		_, _ = io.WriteString(w, "<ListBucketResult><IsTruncated>false</IsTruncated>")
		for _, key := range slices.Sorted(maps.Keys(b.objects)) {
			if strings.HasPrefix(key, prefix) {
				_, _ = fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%d</Size></Contents>", key, len(b.objects[key]))
			}
		}
		_, _ = io.WriteString(w, "</ListBucketResult>")
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		b.objects[key] = string(body)
		b.puts++
	case r.Method == http.MethodGet:
		data, ok := b.objects[key]
		if !ok {
			http.Error(w, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, key, time.Time{}, strings.NewReader(data))
	case r.Method == http.MethodDelete:
		delete(b.objects, key)
		w.WriteHeader(http.StatusNoContent)
	}
}

// keys returns the sorted object keys.
func (b *fakeBucket) keys() []string {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return slices.Sorted(maps.Keys(b.objects))
}

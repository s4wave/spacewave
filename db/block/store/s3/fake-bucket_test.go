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
//
// Without versioning it answers a version listing with 501, as Cloudflare R2
// does. With versioning it lists versions, and an overwrite or a delete
// without a version id hides the current version, as Backblaze B2 does.
type fakeBucket struct {
	mtx     sync.Mutex
	objects map[string]string
	// puts counts the PUT requests.
	puts int
	// lists counts the object listing requests.
	lists int
	// versioned enables versioning.
	versioned bool
	// hidden counts the hidden versions of each key.
	hidden map[string]int
}

// newFakeBucket serves a bucket holding objects until the test ends.
func newFakeBucket(t *testing.T, objects map[string]string) (*fakeBucket, *Client) {
	// Serve the objects until the test ends.
	t.Helper()
	b := &fakeBucket{objects: objects, hidden: make(map[string]int)}
	if b.objects == nil {
		b.objects = make(map[string]string)
	}
	srv := httptest.NewServer(b)
	t.Cleanup(srv.Close)
	return b, newTestClient(t, srv)
}

// ServeHTTP answers one S3 request.
func (b *fakeBucket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Hold the bucket for the request.
	b.mtx.Lock()
	defer b.mtx.Unlock()

	// Answer by the request's key and query.
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	query := r.URL.Query()
	switch {
	case query.Has("versions"):
		if !b.versioned {
			http.Error(w, "<Error><Code>NotImplemented</Code></Error>", http.StatusNotImplemented)
			return
		}
		b.listVersions(w, query.Get("prefix"))
	case query.Get("list-type") == "2":
		b.lists++
		prefix := query.Get("prefix")
		_, _ = io.WriteString(w, "<ListBucketResult><IsTruncated>false</IsTruncated>")
		for _, key := range slices.Sorted(maps.Keys(b.objects)) {
			if strings.HasPrefix(key, prefix) {
				_, _ = fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%d</Size></Contents>", key, len(b.objects[key]))
			}
		}
		_, _ = io.WriteString(w, "</ListBucketResult>")
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		b.hide(key)
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
		b.delete(key, query.Get("versionId"))
		w.WriteHeader(http.StatusNoContent)
	}
}

// listVersions writes a ListVersionsResult of the keys under prefix. The
// current version of a key is "current" and its hidden versions are "hidden".
func (b *fakeBucket) listVersions(w io.Writer, prefix string) {
	_, _ = io.WriteString(w, "<ListVersionsResult><IsTruncated>false</IsTruncated>")
	for key := range b.objects {
		if strings.HasPrefix(key, prefix) {
			_, _ = fmt.Fprintf(w, "<Version><Key>%s</Key><VersionId>current</VersionId></Version>", key)
		}
	}
	for key, count := range b.hidden {
		if strings.HasPrefix(key, prefix) {
			for range count {
				_, _ = fmt.Fprintf(w, "<DeleteMarker><Key>%s</Key><VersionId>hidden</VersionId></DeleteMarker>", key)
			}
		}
	}
	_, _ = io.WriteString(w, "</ListVersionsResult>")
}

// hide keeps the current version of key as a hidden version when the bucket
// is versioned.
func (b *fakeBucket) hide(key string) {
	if _, ok := b.objects[key]; ok && b.versioned {
		b.hidden[key]++
	}
}

// delete removes version of key, or hides the current version when version is
// empty.
func (b *fakeBucket) delete(key, version string) {
	switch version {
	case "":
		b.hide(key)
		delete(b.objects, key)
	case "current":
		delete(b.objects, key)
	case "hidden":
		if b.hidden[key]--; b.hidden[key] <= 0 {
			delete(b.hidden, key)
		}
	}
}

// hiddenVersions returns the number of hidden versions.
func (b *fakeBucket) hiddenVersions() int {
	// Sum the hidden versions of every key.
	b.mtx.Lock()
	defer b.mtx.Unlock()
	var count int
	for _, n := range b.hidden {
		count += n
	}
	return count
}

// keys returns the sorted object keys.
func (b *fakeBucket) keys() []string {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return slices.Sorted(maps.Keys(b.objects))
}

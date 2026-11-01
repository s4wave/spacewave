// Package s3test serves an in-memory S3 bucket for tests of code that writes
// objects: PUT, ranged GET and HEAD, and ListObjectsV2 by prefix.
package s3test

import (
	"encoding/xml"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// BucketName is the name of the bucket a Bucket serves.
const BucketName = "bucket"

// Bucket is an in-memory S3 bucket named BucketName.
type Bucket struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// objects are the stored objects by key.
	objects map[string][]byte
	// puts counts the accepted PUT requests.
	puts int
	// failPut, when set, decides whether to refuse a PUT of the key with a
	// server error before it is stored.
	failPut func(key string) bool
}

// New serves an empty bucket until the test ends and returns it with a client
// that reaches it.
func New(t *testing.T) (*Bucket, *block_store_s3.Client) {
	// Serve the bucket on a loopback port.
	t.Helper()
	b := &Bucket{objects: make(map[string][]byte)}
	srv := httptest.NewServer(b)
	t.Cleanup(srv.Close)
	client, err := block_store_s3.BuildClient(&block_store_s3.ClientConfig{
		Endpoint:    strings.TrimPrefix(srv.URL, "http://"),
		DisableSsl:  true,
		Credentials: &block_store_s3.Credentials{AccessKeyId: "key", SecretAccessKey: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b, client
}

// FailPut makes the bucket refuse every PUT of a key fail returns true for,
// with a server error, until it is called again. A nil fail accepts all.
func (b *Bucket) FailPut(fail func(key string) bool) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.failPut = fail
}

// Keys returns the sorted object keys under prefix.
func (b *Bucket) Keys(prefix string) []string {
	// List the keys under the lock.
	b.mtx.Lock()
	defer b.mtx.Unlock()
	var keys []string
	for _, key := range slices.Sorted(maps.Keys(b.objects)) {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys
}

// Puts returns the number of PUT requests accepted.
func (b *Bucket) Puts() int {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return b.puts
}

// listResult is a ListObjectsV2 response.
type listResult struct {
	// XMLName names the response element.
	XMLName xml.Name `xml:"ListBucketResult"`
	// IsTruncated reports that more objects remain; the bucket never truncates.
	IsTruncated bool `xml:"IsTruncated"`
	// Contents are the listed objects.
	Contents []listContent `xml:"Contents"`
}

// listContent is one listed object.
type listContent struct {
	// Key is the object key.
	Key string `xml:"Key"`
	// Size is the object size in bytes.
	Size int `xml:"Size"`
}

// ServeHTTP answers one S3 request.
func (b *Bucket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Serve one request at a time.
	b.mtx.Lock()
	defer b.mtx.Unlock()

	// Answer by the request's method, key and query.
	key := strings.TrimPrefix(r.URL.Path, "/"+BucketName+"/")
	query := r.URL.Query()
	switch {
	case query.Get("list-type") == "2":
		prefix := query.Get("prefix")
		var result listResult
		for _, key := range slices.Sorted(maps.Keys(b.objects)) {
			if strings.HasPrefix(key, prefix) {
				result.Contents = append(result.Contents, listContent{Key: key, Size: len(b.objects[key])})
			}
		}
		_ = xml.NewEncoder(w).Encode(result)
	case r.Method == http.MethodPut:
		if b.failPut != nil && b.failPut(key) {
			http.Error(w, "<Error><Code>InternalError</Code></Error>", http.StatusInternalServerError)
			return
		}
		body, _ := io.ReadAll(r.Body)
		b.objects[key] = body
		b.puts++
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		data, ok := b.objects[key]
		if !ok {
			http.Error(w, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, key, time.Time{}, strings.NewReader(string(data)))
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

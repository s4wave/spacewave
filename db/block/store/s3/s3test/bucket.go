// Package s3test serves an in-memory S3 bucket for tests of code that writes
// objects: PUT, ranged GET and HEAD, ListObjectsV2 by prefix, and the versions
// of an overwritten key.
package s3test

import (
	"encoding/xml"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
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
	// history holds the versions of each key, newest first.
	history map[string][]objectVersion
	// versions counts the versions stored, which names the next one.
	versions int
	// puts counts the accepted PUT requests.
	puts int
	// failPut, when set, decides whether to refuse a PUT of the key with a
	// server error before it is stored.
	failPut func(key string) bool
	// url is where the bucket serves.
	url string
	// uploads counts the multipart uploads begun.
	uploads int
	// parts holds each open multipart upload's parts by number.
	parts map[string]map[int][]byte
}

// objectVersion is one stored version of a key.
type objectVersion struct {
	// id names the version.
	id string
	// data is its content.
	data []byte
}

// New serves an empty bucket until the test ends and returns it with a client
// that reaches it.
func New(t *testing.T) (*Bucket, *block_store_s3.Client) {
	// Serve the bucket on a loopback port.
	t.Helper()
	b := &Bucket{objects: make(map[string][]byte), history: make(map[string][]objectVersion), parts: make(map[string]map[int][]byte)}
	srv := httptest.NewServer(b)
	t.Cleanup(srv.Close)
	b.url = srv.URL
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

// URL returns the endpoint the bucket serves on.
func (b *Bucket) URL() string {
	return b.url
}

// OpenUploads returns the multipart uploads neither completed nor aborted.
func (b *Bucket) OpenUploads() int {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return len(b.parts)
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
	case query.Has("versions"):
		b.listVersions(w, query.Get("prefix"))
	case query.Get("list-type") == "2":
		prefix := query.Get("prefix")
		var result listResult
		for _, key := range slices.Sorted(maps.Keys(b.objects)) {
			if strings.HasPrefix(key, prefix) {
				result.Contents = append(result.Contents, listContent{Key: key, Size: len(b.objects[key])})
			}
		}
		_ = xml.NewEncoder(w).Encode(result)
	case r.Method == http.MethodPost && query.Has("uploads"):
		b.uploads++
		id := strconv.Itoa(b.uploads)
		b.parts[id] = make(map[int][]byte)
		_, _ = io.WriteString(w, "<InitiateMultipartUploadResult><UploadId>"+id+"</UploadId></InitiateMultipartUploadResult>")
	case r.Method == http.MethodPut && query.Has("uploadId"):
		if b.failPut != nil && b.failPut(key) {
			http.Error(w, "<Error><Code>InternalError</Code></Error>", http.StatusInternalServerError)
			return
		}
		number, _ := strconv.Atoi(query.Get("partNumber"))
		body, _ := io.ReadAll(r.Body)
		b.parts[query.Get("uploadId")][number] = body
		w.Header().Set("ETag", `"`+query.Get("partNumber")+`"`)
	case r.Method == http.MethodPost && query.Has("uploadId"):
		parts := b.parts[query.Get("uploadId")]
		var object []byte
		for _, number := range slices.Sorted(maps.Keys(parts)) {
			object = append(object, parts[number]...)
		}
		b.store(key, object)
		delete(b.parts, query.Get("uploadId"))
		_, _ = io.WriteString(w, "<CompleteMultipartUploadResult/>")
	case r.Method == http.MethodDelete && query.Has("uploadId"):
		delete(b.parts, query.Get("uploadId"))
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut:
		if b.failPut != nil && b.failPut(key) {
			http.Error(w, "<Error><Code>InternalError</Code></Error>", http.StatusInternalServerError)
			return
		}
		body, _ := io.ReadAll(r.Body)
		b.store(key, body)
		b.puts++
	case r.Method == http.MethodDelete && query.Has("versionId"):
		b.deleteVersion(key, query.Get("versionId"))
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		data, ok := b.objects[key]
		if version := query.Get("versionId"); version != "" {
			i := slices.IndexFunc(b.history[key], func(v objectVersion) bool { return v.id == version })
			if ok = i >= 0; ok {
				data = b.history[key][i].data
			}
		}
		if !ok {
			http.Error(w, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, key, time.Time{}, strings.NewReader(string(data)))
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// store makes data the current version of key.
func (b *Bucket) store(key string, data []byte) {
	b.versions++
	version := objectVersion{id: strconv.Itoa(b.versions), data: data}
	b.objects[key] = data
	b.history[key] = slices.Insert(b.history[key], 0, version)
}

// deleteVersion removes one version of key, so the next newest becomes current.
func (b *Bucket) deleteVersion(key, id string) {
	// Remove the version from the history.
	history := slices.DeleteFunc(b.history[key], func(v objectVersion) bool { return v.id == id })
	b.history[key] = history

	// Serve the newest version left, or nothing.
	if len(history) == 0 {
		delete(b.objects, key)
		return
	}
	b.objects[key] = history[0].data
}

// listVersions writes a ListVersionsResult of the keys under prefix, each key's
// versions newest first.
func (b *Bucket) listVersions(w io.Writer, prefix string) {
	_, _ = io.WriteString(w, "<ListVersionsResult><IsTruncated>false</IsTruncated>")
	for _, key := range slices.Sorted(maps.Keys(b.history)) {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		for _, version := range b.history[key] {
			_, _ = io.WriteString(w, "<Version><Key>"+key+"</Key><VersionId>"+version.id+"</VersionId></Version>")
		}
	}
	_, _ = io.WriteString(w, "</ListVersionsResult>")
}

// Package s3test serves an in-memory S3 bucket for tests of code that writes
// objects: PUT, copy of a key or of one version, ranged GET and HEAD,
// ListObjectsV2 by prefix, and the versions of an overwritten or hidden key.
// A delete with a version id removes that version; one without hides the key
// under a delete marker, as a versioned bucket does. The bucket reports its
// versioning, and a bucket with versioning off keeps one version per key.
//
// The same server answers B2's native b2_authorize_account and
// b2_list_buckets, which report the bucket's lifecycle rules, the one place
// B2 shows a rule that hides or deletes versions on its own.
package s3test

import (
	"crypto/md5" //nolint:gosec // S3 defines a single-part ETag as the MD5 of the bytes.
	"encoding/hex"
	"encoding/xml"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
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
	// puts counts the accepted PUT requests that carry a body.
	puts int
	// copies counts the accepted copies.
	copies int
	// deleted counts the content versions deleted, by key, a marker not
	// among them.
	deleted map[string]int
	// marked holds each key that has carried a delete marker.
	marked map[string]bool
	// failPut, when set, decides whether to refuse a PUT of the key with a
	// server error before it is stored.
	failPut func(key string) bool
	// refuseVersions, when set, answers every version listing with 501, as
	// a service without versioning or a key it cannot list does.
	refuseVersions bool
	// unversioned, when set, keeps one version per key and reports
	// versioning off.
	unversioned bool
	// oldestFirst, when set, lists each key's versions oldest first, an
	// order the S3 API does not rule out.
	oldestFirst bool
	// lifecycle is the JSON value b2_list_buckets reports as the bucket's
	// lifecycleRules, "[]" when unset.
	lifecycle string
	// b2Status, when nonzero, is the status every B2 native call answers.
	b2Status int
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
	// modified is when it was stored, served as Last-Modified.
	modified time.Time
	// marker is set on a delete marker, which hides the key and holds no
	// content.
	marker bool
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

// RefuseVersions makes the bucket answer every version listing with 501 Not
// Implemented while refuse is set, as a service without versioning does.
func (b *Bucket) RefuseVersions(refuse bool) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.refuseVersions = refuse
}

// ListOldestFirst makes every version listing give each key's versions
// oldest first while set, as a store that does not keep S3's usual order may.
func (b *Bucket) ListOldestFirst(set bool) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.oldestFirst = set
}

// SetLifecycleRules sets the JSON value b2_list_buckets reports as the
// bucket's lifecycleRules, such as
// `[{"fileNamePrefix":"","daysFromHidingToDeleting":1,"daysFromUploadingToHiding":null}]`
// for B2's "keep only the last version". The fake reports the rules and does
// not run them.
func (b *Bucket) SetLifecycleRules(rules string) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.lifecycle = rules
}

// FailB2 makes every B2 native call answer status, or answer normally again
// when status is zero.
func (b *Bucket) FailB2(status int) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.b2Status = status
}

// SetVersioning turns versioning on, as a new bucket has it, or off, where
// each write replaces the key's history with its one version.
func (b *Bucket) SetVersioning(enabled bool) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.unversioned = !enabled
}

// Hide puts a delete marker on key as its newest version, as a lifecycle rule
// or a plain delete does in a versioned bucket: the key leaves the object
// listing and plain reads, and its older versions stay readable by id.
func (b *Bucket) Hide(key string) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.hide(key)
}

// hide records a delete marker as the newest version of key, as a delete
// without a version id does, and drops the object.
func (b *Bucket) hide(key string) {
	// Without versioning a delete removes the key outright.
	if b.unversioned {
		b.countDeleted(key, b.history[key])
		delete(b.history, key)
		delete(b.objects, key)
		return
	}

	// Otherwise it adds a marker over the versions.
	b.versions++
	b.history[key] = slices.Insert(b.history[key], 0, objectVersion{id: strconv.Itoa(b.versions), modified: time.Now(), marker: true})
	delete(b.objects, key)
	if b.marked == nil {
		b.marked = make(map[string]bool)
	}
	b.marked[key] = true
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

// Copies returns the number of copies accepted.
func (b *Bucket) Copies() int {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return b.copies
}

// Puts returns the number of PUT requests with a body accepted.
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
	case strings.HasPrefix(r.URL.Path, "/b2api/"):
		b.serveB2(w, r)
	case query.Has("versions") && b.refuseVersions:
		http.Error(w, "<Error><Code>NotImplemented</Code></Error>", http.StatusNotImplemented)
	case query.Has("versions"):
		b.listVersions(w, query.Get("prefix"))
	case r.Method == http.MethodGet && query.Has("versioning"):
		status := "<Status>Enabled</Status>"
		if b.unversioned {
			status = ""
		}
		_, _ = io.WriteString(w, "<VersioningConfiguration>"+status+"</VersioningConfiguration>")
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
	case r.Method == http.MethodPut && r.Header.Get("X-Amz-Copy-Source") != "":
		data, ok := b.copySource(r.Header.Get("X-Amz-Copy-Source"))
		if !ok {
			http.Error(w, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
			return
		}
		b.store(key, slices.Clone(data))
		b.copies++
		_, _ = io.WriteString(w, "<CopyObjectResult/>")
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
	case r.Method == http.MethodDelete:
		b.hide(key)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		history := b.history[key]
		i := 0
		if version := query.Get("versionId"); version != "" {
			i = slices.IndexFunc(history, func(v objectVersion) bool { return v.id == version })
			if i >= 0 && history[i].marker {
				http.Error(w, "<Error><Code>MethodNotAllowed</Code></Error>", http.StatusMethodNotAllowed)
				return
			}
		} else if len(history) != 0 && history[0].marker {
			i = -1
		}
		if i < 0 || i >= len(history) {
			http.Error(w, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, key, history[i].modified, strings.NewReader(string(history[i].data)))
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// copySource returns the data a copy source names: a key's current version,
// or the version its versionId names, which a delete marker does not hold.
func (b *Bucket) copySource(header string) ([]byte, bool) {
	// Split the key from the version.
	path, query, _ := strings.Cut(header, "?")
	path, _ = url.PathUnescape(path)
	key := strings.TrimPrefix(path, "/"+BucketName+"/")
	values, _ := url.ParseQuery(query)
	version := values.Get("versionId")
	if version == "" {
		data, ok := b.objects[key]
		return data, ok
	}

	// Find the version.
	i := slices.IndexFunc(b.history[key], func(v objectVersion) bool { return v.id == version })
	if i < 0 || b.history[key][i].marker {
		return nil, false
	}
	return b.history[key][i].data, true
}

// ContentDeleted returns the number of content versions of keys under
// prefix the bucket deleted, by a delete naming the version or, without
// versioning, by a delete or an overwrite. A delete marker's removal is not
// counted.
func (b *Bucket) ContentDeleted(prefix string) int {
	// Sum the counts of the keys under the prefix.
	b.mtx.Lock()
	defer b.mtx.Unlock()
	n := 0
	for key, count := range b.deleted {
		if strings.HasPrefix(key, prefix) {
			n += count
		}
	}
	return n
}

// HiddenDeleted returns the keys under prefix that have carried a delete
// marker and lost a content version, with the count each lost, so a check
// that a hide never purges tells them from other deletes in the same run.
func (b *Bucket) HiddenDeleted(prefix string) map[string]int {
	// Collect the marked keys under the prefix that lost a version.
	b.mtx.Lock()
	defer b.mtx.Unlock()
	out := make(map[string]int)
	for key, count := range b.deleted {
		if b.marked[key] && strings.HasPrefix(key, prefix) {
			out[key] = count
		}
	}
	return out
}

// countDeleted counts the content versions of versions, deleted from key.
func (b *Bucket) countDeleted(key string, versions []objectVersion) {
	for _, v := range versions {
		if v.marker {
			continue
		}
		if b.deleted == nil {
			b.deleted = make(map[string]int)
		}
		b.deleted[key]++
	}
}

// Versions returns each version under prefix as the listing reports it, by
// key, newest first: the key, and a marker flag or the content.
func (b *Bucket) Versions(prefix string) []string {
	// Hold the bucket while reading its history.
	b.mtx.Lock()
	defer b.mtx.Unlock()

	// Describe each version of each key under the prefix, in key order.
	var out []string
	for _, key := range slices.Sorted(maps.Keys(b.history)) {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		for _, version := range b.history[key] {
			if version.marker {
				out = append(out, key+" hidden")
				continue
			}
			out = append(out, key+" "+string(version.data))
		}
	}
	return out
}

// store makes data the current version of key.
func (b *Bucket) store(key string, data []byte) {
	// Make the data the newest version, or the only one without versioning.
	b.versions++
	version := objectVersion{id: strconv.Itoa(b.versions), data: data, modified: time.Now()}
	b.objects[key] = data
	if b.unversioned {
		b.countDeleted(key, b.history[key])
		b.history[key] = nil
	}
	b.history[key] = slices.Insert(b.history[key], 0, version)
}

// deleteVersion removes one version of key, so the next newest becomes current.
func (b *Bucket) deleteVersion(key, id string) {
	// Remove the version from the history.
	history := slices.DeleteFunc(b.history[key], func(v objectVersion) bool {
		if v.id != id {
			return false
		}
		b.countDeleted(key, []objectVersion{v})
		return true
	})
	b.history[key] = history

	// Serve the newest version left, or nothing when none is left or the
	// newest is a delete marker.
	if len(history) == 0 || history[0].marker {
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
		history := slices.Clone(b.history[key])
		if b.oldestFirst {
			slices.Reverse(history)
		}
		for _, version := range history {
			element, etag := "DeleteMarker", ""
			if !version.marker {
				sum := md5.Sum(version.data) //nolint:gosec // S3 defines a single-part ETag as the MD5 of the bytes.
				element, etag = "Version", "<ETag>&quot;"+hex.EncodeToString(sum[:])+"&quot;</ETag>"
			}
			latest := strconv.FormatBool(version.id == b.history[key][0].id)
			modified := version.modified.UTC().Format("2006-01-02T15:04:05.000Z")
			_, _ = io.WriteString(w, "<"+element+"><Key>"+key+"</Key><VersionId>"+version.id+"</VersionId><IsLatest>"+latest+"</IsLatest><LastModified>"+modified+"</LastModified>"+etag+"<Size>"+strconv.Itoa(len(version.data))+"</Size></"+element+">")
		}
	}
	_, _ = io.WriteString(w, "</ListVersionsResult>")
}

// serveB2 answers B2's native b2_authorize_account, with the server itself as
// the apiUrl, and b2_list_buckets, which reports the bucket and its lifecycle
// rules to a request that names BucketName or no bucket.
func (b *Bucket) serveB2(w http.ResponseWriter, r *http.Request) {
	if b.b2Status != 0 {
		http.Error(w, `{"status":`+strconv.Itoa(b.b2Status)+`,"code":"fake"}`, b.b2Status)
		return
	}
	switch r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:] {
	case "b2_authorize_account":
		if _, _, ok := r.BasicAuth(); !ok {
			http.Error(w, `{"status":401,"code":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"accountId":"account","authorizationToken":"token","apiInfo":{"storageApi":{"apiUrl":"`+b.url+`"}}}`)
	case "b2_list_buckets":
		body, _ := io.ReadAll(r.Body)
		var parser fastjson.Parser
		req, err := parser.ParseBytes(body)
		if r.Header.Get("Authorization") != "token" || err != nil || string(req.GetStringBytes("accountId")) != "account" {
			http.Error(w, `{"status":401,"code":"bad_auth_token"}`, http.StatusUnauthorized)
			return
		}
		if name := string(req.GetStringBytes("bucketName")); name != "" && name != BucketName {
			_, _ = io.WriteString(w, `{"buckets":[]}`)
			return
		}
		rules := b.lifecycle
		if rules == "" {
			rules = "[]"
		}
		_, _ = io.WriteString(w, `{"buckets":[{"bucketName":"`+BucketName+`","lifecycleRules":`+rules+`}]}`)
	default:
		http.Error(w, `{"status":404,"code":"not_found"}`, http.StatusNotFound)
	}
}

//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeMultipart is an in-memory S3 bucket named "bucket" serving PutObject and
// the four multipart upload calls for one upload at a time.
type fakeMultipart struct {
	mtx sync.Mutex
	// objects holds the stored objects by key.
	objects map[string][]byte
	// parts holds the stored parts of the open upload by part number.
	parts map[int][]byte
	// completed lists the part numbers of the last complete request in order.
	completed []int
	// aborts counts the abort requests.
	aborts int
	// failPart is a part number the bucket refuses with 403, or zero.
	failPart int
	// inflight counts the part uploads in progress.
	inflight int
	// maxInflight is the most part uploads seen in progress at once.
	maxInflight int
}

// ServeHTTP answers one S3 request.
func (b *fakeMultipart) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Read the request's key, query and body.
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	query := r.URL.Query()
	body, _ := io.ReadAll(r.Body)

	// Hold each part upload open briefly so concurrent parts overlap.
	if query.Has("partNumber") {
		b.mtx.Lock()
		b.inflight++
		b.maxInflight = max(b.maxInflight, b.inflight)
		b.mtx.Unlock()
		time.Sleep(20 * time.Millisecond)
	}

	// Answer by the request's method and query.
	b.mtx.Lock()
	defer b.mtx.Unlock()
	switch {
	case r.Method == http.MethodPost && query.Has("uploads"):
		b.parts = make(map[int][]byte)
		_, _ = io.WriteString(w, "<InitiateMultipartUploadResult><UploadId>up&amp;1</UploadId></InitiateMultipartUploadResult>")
	case r.Method == http.MethodPut && query.Has("partNumber"):
		b.inflight--
		number, _ := strconv.Atoi(query.Get("partNumber"))
		if number == b.failPart || query.Get("uploadId") != "up&1" {
			http.Error(w, "<Error><Code>AccessDenied</Code></Error>", http.StatusForbidden)
			return
		}
		b.parts[number] = body
		w.Header().Set("ETag", `"etag-`+strconv.Itoa(number)+`"`)
	case r.Method == http.MethodPost && query.Has("uploadId"):
		b.complete(w, key, string(body))
	case r.Method == http.MethodDelete && query.Has("uploadId"):
		b.aborts++
		b.parts = nil
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut:
		b.objects[key] = body
	}
}

// complete joins the parts the request lists into the object at key, checking
// each part's ETag.
func (b *fakeMultipart) complete(w http.ResponseWriter, key, doc string) {
	// Collect the listed parts in request order.
	var data []byte
	b.completed = nil
	err := visitElements(doc, "Part", func(part string) error {
		number, _ := strconv.Atoi(xmlElementText(part, "PartNumber"))
		if xmlElementText(part, "ETag") != `&#34;etag-`+strconv.Itoa(number)+`&#34;` {
			return io.ErrUnexpectedEOF
		}
		b.completed = append(b.completed, number)
		data = append(data, b.parts[number]...)
		return nil
	})
	if err != nil {
		http.Error(w, "<Error><Code>InvalidPart</Code></Error>", http.StatusBadRequest)
		return
	}

	// Store the object.
	b.objects[key] = data
	_, _ = io.WriteString(w, "<CompleteMultipartUploadResult></CompleteMultipartUploadResult>")
}

// TestUploadStream uploads a stream of several parts concurrently and checks
// the stored bytes and part order, then checks that a refused part aborts the
// upload and that a stream shorter than one part is a single PutObject.
func TestUploadStream(t *testing.T) {
	// Serve the fake bucket.
	ctx := context.Background()
	b := &fakeMultipart{objects: make(map[string][]byte)}
	srv := httptest.NewServer(b)
	defer srv.Close()
	client := newTestClient(t, srv)

	// Build a stream of three full parts and a short tail.
	data := make([]byte, 3*MinPartSize+1234)
	for i := range data {
		data[i] = byte(i * 7 / 13)
	}

	// Upload it through a reader that hides its length, counting progress.
	var progress atomic.Int64
	opts := &UploadStreamOptions{
		PartSize:    MinPartSize,
		Concurrency: 3,
		Progress:    func(n int) { progress.Add(int64(n)) },
	}
	n, err := client.UploadStream(ctx, "bucket", "big", io.MultiReader(bytes.NewReader(data)), opts)
	if err != nil {
		t.Fatal(err)
	}

	// Check the stored bytes, the part order, the overlap and the progress.
	if n != int64(len(data)) || !bytes.Equal(b.objects["big"], data) {
		t.Fatalf("stored %d of %d bytes, equal %v", n, len(data), bytes.Equal(b.objects["big"], data))
	}
	if got := b.completed; len(got) != 4 || got[0] != 1 || got[1] != 2 || got[2] != 3 || got[3] != 4 {
		t.Fatalf("completed parts %v", got)
	}
	if b.maxInflight < 2 {
		t.Fatalf("at most %d parts uploaded at once", b.maxInflight)
	}
	if progress.Load() != int64(len(data)) {
		t.Fatalf("progress %d of %d", progress.Load(), len(data))
	}

	// Refuse part 2 of the next upload and check that it aborts.
	b.failPart = 2
	if _, err := client.UploadStream(ctx, "bucket", "failed", bytes.NewReader(data), opts); err == nil {
		t.Fatal("upload with a refused part succeeded")
	}
	if _, ok := b.objects["failed"]; ok || b.aborts != 1 {
		t.Fatalf("refused upload stored %v with %d aborts", ok, b.aborts)
	}

	// Upload a stream shorter than one part as one object.
	if _, err := client.UploadStream(ctx, "bucket", "small", strings.NewReader("tiny"), opts); err != nil {
		t.Fatal(err)
	}
	if string(b.objects["small"]) != "tiny" || b.aborts != 1 {
		t.Fatalf("small object %q", b.objects["small"])
	}
}

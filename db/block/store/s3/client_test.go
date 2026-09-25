//go:build !tinygo

package block_store_s3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// TestListObjectsFollowsContinuation lists a listing split across two pages
// and sends the first page's continuation token, including its reserved
// characters, with the second request.
func TestListObjectsFollowsContinuation(t *testing.T) {
	const token = "1ueGcxLPRx1Tr/XYExHnhbYLgveDs2J/wm36Hy4vbOwM="
	pages := map[string]string{
		"": `<ListBucketResult><IsTruncated>true</IsTruncated>` +
			`<Contents><Key>p/a&amp;x</Key><Size>10</Size></Contents>` +
			`<Contents><Key>p/b</Key><Size>20</Size></Contents>` +
			`<NextContinuationToken>` + token + `</NextContinuationToken></ListBucketResult>`,
		token: `<ListBucketResult><IsTruncated>false</IsTruncated>` +
			`<Contents><Key>p/c</Key><Size>5</Size></Contents></ListBucketResult>`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/bucket" || query.Get("list-type") != "2" || query.Get("prefix") != "p/" {
			http.Error(w, "unexpected request "+r.URL.String(), http.StatusBadRequest)
			return
		}
		page, ok := pages[query.Get("continuation-token")]
		if !ok {
			http.Error(w, "unknown token", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, page)
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	var keys []string
	var size int64
	err := client.ListObjects(context.Background(), "bucket", "p/", func(key string, objectSize int64) error {
		keys = append(keys, key)
		size += objectSize
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(keys, []string{"p/a&x", "p/b", "p/c"}) || size != 35 {
		t.Fatalf("listed %v with %d bytes; want [p/a&x p/b p/c] with 35 bytes", keys, size)
	}
}

// TestDoRetriesTransientStatus retries a PUT the service fails with a 500 and
// sends the same body again.
func TestDoRetriesTransientStatus(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if len(bodies) == 1 {
			http.Error(w, "<Error><Code>InternalError</Code></Error>", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	err := client.PutObject(context.Background(), "bucket", "key", []byte("data"), "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(bodies, []string{"data", "data"}) {
		t.Fatalf("bodies = %q; want the body sent twice", bodies)
	}
}

// TestCheckBucketDeletesStaleProbes leaves the probe of an interrupted check
// out of the usage and deletes it.
func TestCheckBucketDeletesStaleProbes(t *testing.T) {
	objects := map[string]string{
		"p/block":                  "0123456789",
		"p/.spacewave-check/stale": "probe",
	}
	bucket, client := newFakeBucket(t, objects)
	result := CheckBucket(context.Background(), client, "bucket", "p/")
	if result.GetOutcome() != CheckOutcome_CHECK_OUTCOME_OK {
		t.Fatalf("check = %v: %s", result.GetOutcome(), result.GetDetail())
	}
	if result.GetUsage().GetObjects() != 1 || result.GetUsage().GetBytes() != 10 {
		t.Fatalf("usage = %v; want 1 object of 10 bytes", result.GetUsage())
	}
	if keys := bucket.keys(); !slices.Equal(keys, []string{"p/block"}) {
		t.Fatalf("objects = %v; want only p/block", keys)
	}
}

// newTestClient builds a client for the test server.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	client, err := BuildClient(&ClientConfig{
		Endpoint:    strings.TrimPrefix(srv.URL, "http://"),
		DisableSsl:  true,
		Credentials: &Credentials{AccessKeyId: "key", SecretAccessKey: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

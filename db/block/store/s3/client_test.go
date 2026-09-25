//go:build !tinygo

package block_store_s3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSumObjectsFollowsContinuation sums a listing split across two pages and
// sends the first page's continuation token, including its reserved
// characters, with the second request.
func TestSumObjectsFollowsContinuation(t *testing.T) {
	const token = "1ueGcxLPRx1Tr/XYExHnhbYLgveDs2J/wm36Hy4vbOwM="
	pages := map[string]string{
		"": `<ListBucketResult><IsTruncated>true</IsTruncated>` +
			`<Contents><Key>p/a</Key><Size>10</Size></Contents>` +
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

	client, err := BuildClient(&ClientConfig{
		Endpoint:    strings.TrimPrefix(srv.URL, "http://"),
		DisableSsl:  true,
		Credentials: &Credentials{AccessKeyId: "key", SecretAccessKey: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	usage, err := client.SumObjects(context.Background(), "bucket", "p/")
	if err != nil {
		t.Fatal(err)
	}
	if usage.GetObjects() != 3 || usage.GetBytes() != 35 {
		t.Fatalf("usage = %d objects, %d bytes; want 3 objects, 35 bytes", usage.GetObjects(), usage.GetBytes())
	}
}

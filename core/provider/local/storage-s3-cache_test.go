package provider_local

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/sirupsen/logrus"
)

// TestS3ReadCacheBinding proves the provider mounts the optional range wrapper
// without routing uploads or entry listings through the cache.
func TestS3ReadCacheBinding(t *testing.T) {
	// Seed the real S3 test bucket through the provider's normal pack store.
	bucket, _ := s3test.New(t)
	location := &account_settings.S3Location{Endpoint: strings.TrimPrefix(bucket.URL(), "http://"), DisableSsl: true, Bucket: s3test.BucketName, KeepPacks: true}
	creds := &block_store_s3.Credentials{AccessKeyId: "key", SecretAccessKey: "secret"}
	writer, err := buildS3BlockStore(logrus.NewEntry(logrus.New()), location, "world", creds, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	value := []byte("the provider reads this block through its optional cache")
	ref, _, err := writer.PutBlock(t.Context(), value, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Serve fake cache reads from the same bucket, with the Worker's framing.
	var calls atomic.Int32
	cacheServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/packs/") || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("cache received a non-pack range read")
		}
		w.Header().Set("X-Cache", "HIT")
		bucket.ServeHTTP(w, r)
	}))
	defer cacheServer.Close()
	cache := block_store_s3.NewHTTPRangeCache(cacheServer.URL, "test-token")
	SetS3ReadCache(location.GetEndpoint(), location.GetBucket(), "world", cache)
	defer SetS3ReadCache("", "", "", nil)

	// Reopen so no resident pack bytes can hide missing provider wiring.
	reader, err := buildS3BlockStore(logrus.NewEntry(logrus.New()), location, "world", creds, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, found, err := reader.GetBlock(t.Context(), ref)
	if err != nil || !found || !bytes.Equal(data, value) {
		t.Fatalf("read: %q %v %v", data, found, err)
	}
	if calls.Load() == 0 {
		t.Fatal("provider never called the optional cache")
	}
}

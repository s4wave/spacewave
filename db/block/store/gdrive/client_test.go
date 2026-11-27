//go:build !tinygo

package block_store_gdrive_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_gdrive "github.com/s4wave/spacewave/db/block/store/gdrive"
	"github.com/s4wave/spacewave/db/block/store/gdrive/gdrivetest"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/sirupsen/logrus"
)

// TestImmutableObjects exercises multipart, resumable chunks, paged listing,
// range reads and reopening over the fake service, including a refused delete.
func TestImmutableObjects(t *testing.T) {
	// Open an isolated fake, with enough entries to require multiple pages.
	server := gdrivetest.NewServer()
	defer server.Close()
	transport := &faultTransport{base: server.HTTP.Client().Transport, server: server, loseFolder: true}
	client, err := block_store_gdrive.NewClient(&http.Client{Transport: transport}, "root", server.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	for i, size := range []int{1, 1 << 10, 8 << 20, (8 << 20) + 17} {
		key := "space/packs/" + strconv.Itoa(i)
		data := bytes.Repeat([]byte{byte(i + 1)}, size)
		if err := client.PutObject(t.Context(), "", key, data, "application/octet-stream"); err != nil {
			t.Fatal(err)
		}
		got, err := client.GetObjectRange(t.Context(), "", key, 0, min(size, 32))
		if err != nil || !bytes.Equal(got, data[:min(size, 32)]) {
			t.Fatalf("range: %x, %v", got, err)
		}
	}

	// Two concurrent directory creates can leave disjoint packs under one name.
	if !server.SplitFolder("packs") {
		t.Fatal("fixture had no packs folder to split")
	}

	// Reopening must choose verified files from the persisted metadata alone.
	reader, err := block_store_gdrive.NewClient(server.HTTP.Client(), "root", server.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	if err := reader.ListObjects(t.Context(), "", "space/packs/", func(key string, size int64) error { count++; return nil }); err != nil || count != 4 {
		t.Fatalf("listed %d files: %v", count, err)
	}
	body, err := reader.GetObject(t.Context(), "", "space/packs/0")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body)
	_ = body.Close() // The entire response has already been read.
	if err != nil || !bytes.Equal(got, []byte{1}) {
		t.Fatalf("read: %x, %v", got, err)
	}

	// Mutation and deletion must be rejected without an HTTP delete or patch.
	if err := reader.PutObject(t.Context(), "", "space/packs/0", []byte{2}, "application/octet-stream"); !errors.Is(err, block_store_gdrive.ErrImmutable) {
		t.Fatalf("overwrite: %v", err)
	}
	if err := reader.DeleteObject(t.Context(), "", "space/packs/0"); !errors.Is(err, block_store_gdrive.ErrDeleteForbidden) {
		t.Fatalf("delete: %v", err)
	}
	for _, call := range server.Calls() {
		if call.Method == http.MethodDelete || call.Method == http.MethodPatch {
			t.Fatalf("backup mutated Drive: %+v", call)
		}
	}
}

// TestDriveOutcomes checks transient backoff and the named waiting outcomes.
func TestDriveOutcomes(t *testing.T) {
	// Retry transient reads through Google's nested error shape.
	server := gdrivetest.NewServer()
	defer server.Close()
	client, err := block_store_gdrive.NewClient(server.HTTP.Client(), "root", server.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	server.Inject(gdrivetest.Failure{Status: 403, Reason: "rateLimitExceeded"}, gdrivetest.Failure{Status: 429}, gdrivetest.Failure{Status: 503})
	if err := client.ListObjects(t.Context(), "", "", func(string, int64) error { return nil }); err != nil {
		t.Fatal(err)
	}

	// Fatal quota and sign-in answers settle rather than entering retry loops.
	for _, tc := range []struct {
		failure gdrivetest.Failure
		want    error
	}{
		{gdrivetest.Failure{Status: 403, Reason: "storageQuotaExceeded"}, block_store_gdrive.ErrQuotaFull},
		{gdrivetest.Failure{Status: 403, Message: "User rate limit exceeded."}, block_store_gdrive.ErrUploadLimit},
		{gdrivetest.Failure{Status: 401}, block_store_gdrive.ErrSignIn},
	} {
		server.Inject(tc.failure)
		if err := client.PutObject(t.Context(), "", "item", []byte("ciphertext"), "application/octet-stream"); !errors.Is(err, tc.want) {
			t.Fatalf("outcome: %v, want %v", err, tc.want)
		}
	}
	if _, err := client.GetObject(t.Context(), "", "missing"); !errors.Is(err, block_store_s3.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

// TestPackStoreRoundTrip checks the PackStore contract over the Drive adapter.
func TestPackStoreRoundTrip(t *testing.T) {
	// Write a pack with a parent and child through the real PackStore.
	server := gdrivetest.NewServer()
	defer server.Close()
	client, err := block_store_gdrive.NewClient(server.HTTP.Client(), "root", server.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	writer := block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), client, "", "space/", &block_store_s3.PackStoreOptions{KeepPacks: true, SoleWriter: true})
	defer writer.Close()
	child, err := block.BuildBlockRef([]byte("ciphertext child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := block.BuildBlockRef([]byte("ciphertext parent"), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = writer.PutBlockBatch(t.Context(), []*block.PutBatchEntry{
		{Ref: child, Data: []byte("ciphertext child"), RefsKnown: true},
		{Ref: parent, Data: []byte("ciphertext parent"), Refs: []*block.BlockRef{child}, RefsKnown: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// A new client must discover complete entries and read the pack by range.
	fresh, err := block_store_gdrive.NewClient(server.HTTP.Client(), "root", server.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	reader := block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), fresh, "", "space/", &block_store_s3.PackStoreOptions{KeepPacks: true, ReadOnlyIndex: true, SoleWriter: true})
	defer reader.Close()
	stored, err := reader.GetStoredBlock(t.Context(), parent)
	if err != nil || stored == nil || string(stored.Data) != "ciphertext parent" || len(stored.Refs) != 1 || !stored.Refs[0].EqualsRef(child) {
		t.Fatalf("restored parent: %v, %v", stored, err)
	}
	for _, call := range server.Calls() {
		if call.Method == http.MethodDelete {
			t.Fatalf("PackStore deleted from Drive: %+v", call)
		}
	}
}

// TestResumableFaults loses each chunk's reply and expires each session position;
// a successful retry must expose exactly the original complete bytes.
func TestResumableFaults(t *testing.T) {
	// A three-chunk object exercises aligned intermediate and short final chunks.
	data := bytes.Repeat([]byte("encrypted"), (2*block_store_gdrive.ChunkBytes)/9+1)
	for _, expire := range []bool{false, true} {
		for at := 1; at <= 3; at++ {
			t.Run(strconv.FormatBool(expire)+"-"+strconv.Itoa(at), func(t *testing.T) {
				// Interrupt one chunk on an otherwise ordinary HTTP transport.
				server := gdrivetest.NewServer()
				defer server.Close()
				transport := &faultTransport{base: server.HTTP.Client().Transport, server: server, at: at, expire: expire}
				client, err := block_store_gdrive.NewClient(&http.Client{Transport: transport}, "root", server.HTTP.URL)
				if err != nil {
					t.Fatal(err)
				}
				if err := client.PutObject(t.Context(), "", "packs/large", data, "application/octet-stream"); err != nil {
					t.Fatal(err)
				}

				// Reopen from persisted Drive metadata rather than the writer's cache.
				reader, err := block_store_gdrive.NewClient(server.HTTP.Client(), "root", server.HTTP.URL)
				if err != nil {
					t.Fatal(err)
				}
				body, err := reader.GetObject(t.Context(), "", "packs/large")
				if err != nil {
					t.Fatal(err)
				}
				got, err := io.ReadAll(body)
				_ = body.Close() // The response is fully consumed before comparing.
				if err != nil || !bytes.Equal(got, data) || transport.at != 0 {
					t.Fatalf("resumed upload: bytes=%d, fired=%v, error=%v", len(got), transport.at == 0, err)
				}
			})
		}
	}

	// Transient chunk replies must delay even the status probe, then resume safely.
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run("retry-after-"+strconv.Itoa(status), func(t *testing.T) {
			server := gdrivetest.NewServer()
			defer server.Close()
			transport := &faultTransport{base: server.HTTP.Client().Transport, server: server, at: 2, status: status}
			client, err := block_store_gdrive.NewClient(&http.Client{Transport: transport}, "root", server.HTTP.URL)
			if err != nil {
				t.Fatal(err)
			}
			if err := client.PutObject(t.Context(), "", "packs/large", data, "application/octet-stream"); err != nil {
				t.Fatal(err)
			}
			if transport.at != 0 || transport.retryDelay < time.Second {
				t.Fatalf("chunk Retry-After ignored: fired=%v delay=%s", transport.at == 0, transport.retryDelay)
			}

			// Verify the completed bytes after the rejected middle chunk resumed.
			body, err := client.GetObject(t.Context(), "", "packs/large")
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(body)
			_ = body.Close() // The complete response has been consumed.
			if err != nil || !bytes.Equal(got, data) {
				t.Fatalf("resumed throttled upload: bytes=%d, %v", len(got), err)
			}
		})
	}
}

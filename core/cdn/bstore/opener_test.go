package cdn_bstore

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestAnonymousOpenerSparseReads bounds transfer amplification for isolated
// payload reads while retaining exact bytes across a larger contiguous read.
func TestAnonymousOpenerSparseReads(t *testing.T) {
	// Serve immutable bytes through real HTTP range handling.
	data := bytes.Repeat([]byte("public range payload"), 1<<19)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "pack.kvf", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(server.Close)
	reader, err := NewAnonymousOpener(server.Client(), server.URL, testSpaceID)("payload", int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reader.Close)
	source := reader.ReaderAt(t.Context())

	// Distant small reads must not each download a megabyte of unrelated content.
	buffer := make([]byte, 4096)
	for _, offset := range []int64{17000, 7<<20 + 23000} {
		if _, err := source.ReadAt(buffer, offset); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buffer, data[offset:offset+int64(len(buffer))]) {
			t.Fatalf("incorrect sparse bytes at %d", offset)
		}
	}
	if transferred := reader.SnapshotStats().RangeResponseBytes; transferred > 512<<10 {
		t.Fatalf("two sparse reads transferred %d bytes, want at most 512 KiB", transferred)
	}

	// A contiguous request larger than the cold window still reads completely.
	buffer = make([]byte, 2<<20)
	if _, err := source.ReadAt(buffer, 2<<20); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buffer, data[2<<20:4<<20]) {
		t.Fatal("incorrect contiguous bytes")
	}
}

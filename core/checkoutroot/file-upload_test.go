//go:build !js

package checkoutroot

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
)

// TestPublishExclusiveUpload checks that only the first complete file claims the name.
func TestPublishExclusiveUpload(t *testing.T) {
	// Publish the first upload inside the pinned root.
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	journal := newUploadJournal(hashmap.NewHashmapKvtx(hashmap.NewBTreeMap[[]byte]()), "test")
	var publications sync.Mutex
	start := &unixfs_rpc.OpsMknodWithContentStart{
		Name: "file", TotalSize: 5, Timestamp: timestamppb.Now(),
		Expect: &unixfs_rpc.OpsMknodWithContentStart_Exclusive{Exclusive: true},
	}
	if err := publishFile(t.Context(), root, journal, &publications, ".", start, strings.NewReader("first")); err != nil {
		t.Fatal(err)
	}

	// Refuse a competing upload and preserve the published bytes.
	err = publishFile(t.Context(), root, journal, &publications, ".", start, strings.NewReader("later"))
	if !errors.Is(err, unixfs_errors.ErrExist) {
		t.Fatalf("second publication = %v, want ErrExist", err)
	}
	data, err := root.ReadFile("file")
	if err != nil || string(data) != "first" {
		t.Fatalf("published file = %q, error %v", data, err)
	}

	// A replacement always uses a name and must clear it after publication.
	start.Expect = nil
	if err := publishFile(t.Context(), root, journal, &publications, ".", start, strings.NewReader("later")); err != nil {
		t.Fatal(err)
	}
	key, _, err := journal.next(t.Context(), "")
	if err != nil || key != "" {
		t.Fatalf("journal after publication: key %q, error %v", key, err)
	}
}

// TestPublishVersionedUpload checks that a host edit invalidates an upload's expectation.
func TestPublishVersionedUpload(t *testing.T) {
	// Read the version of an existing host file.
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	if err := root.WriteFile("file", []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := root.Open("file")
	if err != nil {
		t.Fatal(err)
	}
	version, err := openFileVersion(file)
	closeErr := file.Close()
	if err != nil || closeErr != nil || len(version) == 0 {
		t.Fatalf("opening version = %x, error %v", version, errors.Join(err, closeErr))
	}

	// Edit the host file when the upload reads its body.
	var publications sync.Mutex
	journal := newUploadJournal(hashmap.NewHashmapKvtx(hashmap.NewBTreeMap[[]byte]()), "test")
	start := &unixfs_rpc.OpsMknodWithContentStart{
		Name: "file", TotalSize: 5, Timestamp: timestamppb.Now(),
		Expect: &unixfs_rpc.OpsMknodWithContentStart_ExpectedVersion{ExpectedVersion: version},
	}
	stream := &editingUpload{root: root, data: strings.NewReader("later")}
	err = publishFile(t.Context(), root, journal, &publications, ".", start, stream)
	conflict, ok := errors.AsType[*unixfs_errors.FileVersionConflict](err)
	if !ok {
		t.Fatalf("stale publication = %v, want a version conflict", err)
	}
	current, err := destinationVersion(root, "file")
	if err != nil || !bytes.Equal(current, conflict.Version) || bytes.Equal(current, version) {
		t.Fatalf("current version = %x, conflict version = %x, stale version = %x, error %v", current, conflict.Version, version, err)
	}
	if data, err := root.ReadFile("file"); err != nil || string(data) != "edited" {
		t.Fatalf("host edit = %q, error %v", data, err)
	}

	// A replacement expecting the current version publishes and leaves no journal row.
	start.Expect = &unixfs_rpc.OpsMknodWithContentStart_ExpectedVersion{ExpectedVersion: current}
	if err := publishFile(t.Context(), root, journal, &publications, ".", start, strings.NewReader("later")); err != nil {
		t.Fatal(err)
	}
	if data, err := root.ReadFile("file"); err != nil || string(data) != "later" {
		t.Fatalf("replacement = %q, error %v", data, err)
	}
	if key, _, err := journal.next(t.Context(), ""); err != nil || key != "" {
		t.Fatalf("journal after publication: key %q, error %v", key, err)
	}
}

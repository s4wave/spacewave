//go:build !js

package checkoutroot

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
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
	start := &unixfs_rpc.OpsMknodWithContentStart{
		Name: "file", TotalSize: 5, Timestamp: timestamppb.Now(), Exclusive: true,
	}
	if err := publishFile(t.Context(), root, ".", start, strings.NewReader("first")); err != nil {
		t.Fatal(err)
	}

	// Refuse a competing upload and preserve the published bytes.
	err = publishFile(t.Context(), root, ".", start, strings.NewReader("later"))
	if !errors.Is(err, unixfs_errors.ErrExist) {
		t.Fatalf("second publication = %v, want ErrExist", err)
	}
	data, err := root.ReadFile("file")
	if err != nil || string(data) != "first" {
		t.Fatalf("published file = %q, error %v", data, err)
	}
}

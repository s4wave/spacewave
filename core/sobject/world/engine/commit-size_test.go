package sobject_world_engine_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
)

// TestCommitMaxSizeExceeded checks a commit whose operation exceeds
// sobject.MaxInnerDataSize fails with ErrMaxSizeExceeded, leaves the World
// without its changes, and the engine accepts the next commit.
func TestCommitMaxSizeExceeded(t *testing.T) {
	// Start a World engine on a new Space.
	ctx := t.Context()
	sw := newSpaceWorld(ctx, t)

	// The operation inlines the object key, so it exceeds the limit.
	bigKey := strings.Repeat("k", sobject.MaxInnerDataSize)
	big, err := sw.eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer big.Discard()
	if _, err := big.CreateObject(ctx, bigKey, nil); err != nil {
		t.Fatal(err)
	}
	if err := big.Commit(ctx); !errors.Is(err, sobject.ErrMaxSizeExceeded) {
		t.Fatalf("oversized commit = %v, want %v", err, sobject.ErrMaxSizeExceeded)
	}

	// The next commit succeeds.
	small, err := sw.eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer small.Discard()
	if _, err := small.CreateObject(ctx, "small", nil); err != nil {
		t.Fatal(err)
	}
	if err := small.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The World holds the small object and not the oversized one.
	read, err := sw.eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	for key, want := range map[string]bool{bigKey: false, "small": true} {
		if _, found, err := read.GetObject(ctx, key); err != nil || found != want {
			t.Fatalf("object %.16q found = %v, %v; want %v", key, found, err, want)
		}
	}
}

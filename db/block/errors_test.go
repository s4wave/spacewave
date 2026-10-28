package block

import (
	"errors"
	"strings"
	"testing"
)

// TestNotFoundErrorNamesRef checks that a not-found error names the missing
// block and still matches ErrNotFound.
func TestNotFoundErrorNamesRef(t *testing.T) {
	// Build a ref and its not-found error.
	ref, err := BuildBlockRef([]byte("missing"), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = NotFoundError(ref)

	// It matches ErrNotFound and names the ref.
	if !errors.Is(err, ErrNotFound) || !IsNotAvailable(err) {
		t.Fatalf("%v does not match ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), ref.MarshalString()) {
		t.Fatalf("%v does not name %s", err, ref.MarshalString())
	}
}

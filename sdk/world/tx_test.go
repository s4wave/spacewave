package s4wave_world

import (
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/coord"
)

// TestCommitErrorRestoresStaleGeneration classifies a flattened remote stale
// commit as coord.ErrStaleGeneration and keeps its message.
func TestCommitErrorRestoresStaleGeneration(t *testing.T) {
	remote := errors.New("base SharedObject root is stale: coord: stale generation")
	err := CommitError(remote)
	if !errors.Is(err, coord.ErrStaleGeneration) {
		t.Fatalf("CommitError(%q) does not match ErrStaleGeneration", remote)
	}
	if err.Error() != remote.Error() {
		t.Fatalf("CommitError message = %q, want %q", err.Error(), remote.Error())
	}

	other := errors.New("commit World: context deadline exceeded")
	if err := CommitError(other); err != other {
		t.Fatalf("CommitError(%q) = %v, want unchanged", other, err)
	}
}

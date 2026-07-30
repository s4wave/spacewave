package world_block

import (
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

func TestCoordinatedWriteSnapshotClassificationIsTyped(t *testing.T) {
	if !isCoordinatedWriteSnapshotError(errors.Join(errors.New("backend detail"), kvtx.ErrInvalidSnapshot)) {
		t.Fatal("typed invalid snapshot was not classified")
	}
	if isCoordinatedWriteSnapshotError(block.ErrNotFound) {
		t.Fatal("ordinary missing block was classified as a snapshot")
	}
	if isCoordinatedWriteSnapshotError(errors.New("panic: page 2 already freed")) {
		t.Fatal("diagnostic text was classified as a snapshot")
	}
}

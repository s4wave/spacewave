package forge_lib_kvtx

import (
	"context"
	"strings"
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/bucket"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/s4wave/spacewave/forge/testbed"
)

// TestApplyOpCheckExistsErrors pins the error meaning on both mismatch paths.
func TestApplyOpCheckExistsErrors(t *testing.T) {
	// Start the Forge testbed for key-existence checks.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty key-value store and a writable transaction.
	bls, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	st, err := kvtx_block.NewStore(ctx, tb.Logger, bls, func(*bucket.ObjectRef) error { return nil })
	if err != nil {
		t.Fatal(err.Error())
	}
	btx, err := st.NewKvtxBlockTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer btx.Discard()

	// Stage the handle's writes until the test ends.
	stage, err := tb.WorldState.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer stage.Release()
	handle := forge_target.ExecControllerHandleWithAccess(
		"check-exists-test",
		tb.Volume.GetPeerID(),
		tb.Engine,
		stage.AccessWorldState,
		timestamp.Now(),
	)

	// Set one key so it exists; an empty block ref is sufficient.
	if err := btx.SetCursorAtKey(ctx, []byte("present"), nil, false); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the missing key reports the required-existence failure.
	err = ApplyOpCheckExists(ctx, handle, btx, []byte("missing"), true)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected key-does-not-exist error, got: %v", err)
	}

	// Verify the present key reports the required-absence failure.
	err = ApplyOpCheckExists(ctx, handle, btx, []byte("present"), false)
	if err == nil || !strings.Contains(err.Error(), "exists") || strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected key-exists-unexpectedly error, got: %v", err)
	}
}

//go:build !js

package resource_world

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// failedCommitTx models a storage error before the transaction releases its writer.
type failedCommitTx struct {
	world.Tx
	err error
}

func (tx *failedCommitTx) Commit(context.Context) error { return tx.err }

func TestTxResourceFailedCommitReleasesWriter(t *testing.T) {

	// context ctx.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	tx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("commit failed")
	resource := NewTxResource(nil, nil, &failedCommitTx{Tx: tx, err: failure}, nil, nil)
	defer resource.Release()
	if _, err := resource.Commit(ctx, &s4wave_world.CommitRequest{}); !errors.Is(err, failure) {
		t.Fatalf("commit: %v", err)
	}

	// No client reference release or Discard RPC is needed to admit the next writer.
	nextCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	next, err := tb.Engine.NewTransaction(nextCtx, true)
	if err != nil {
		t.Fatalf("failed commit retained writer: %v", err)
	}
	next.Discard()
}

func TestWriteBudgetSpentRefusal(t *testing.T) {
	// A write whose replays spent the budget reads as a plugin timeout, on
	// the operation and batch paths alike, and never as a superseded write.
	spent := &world.WriteBudgetSpentError{Length: time.Second, Err: world.ErrSuperseded}
	if world.IsSuperseded(spent) {
		t.Fatal("spent budget reads as superseded")
	}
	for _, atomic := range []bool{true, false} {
		code, rejection, ok := opErrorRefusal(spent, atomic)
		if !ok || rejection != nil || code != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_OP_TIMEOUT {
			t.Fatalf("atomic %v: refusal = %v %v %v, want PLUGIN_OP_TIMEOUT", atomic, code, rejection, ok)
		}
	}
	refusal, err := batchOpRefusal(spent)
	if err != nil || refusal.GetErrorCode() != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_OP_TIMEOUT {
		t.Fatalf("batch refusal = %v %v, want PLUGIN_OP_TIMEOUT", refusal, err)
	}
}

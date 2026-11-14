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

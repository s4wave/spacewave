//go:build !js

package resource_world

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// failedCommitTx models a storage error before the transaction releases its writer.
type failedCommitTx struct {
	world.Tx
	err error
	// discards counts Discard calls.
	discards int
}

func (tx *failedCommitTx) Commit(context.Context) error { return tx.err }

// Discard counts the call and discards the wrapped transaction.
func (tx *failedCommitTx) Discard() {
	tx.discards++
	tx.Tx.Discard()
}

// TestTxResourceCommitRefusalRestoredByType sends a commit the World refuses
// across SRPC through CommitRemote, the decode both SDK Txs use, and checks
// that the client restores the refusal by type with the server's message, that
// an unknown outcome arrives as ErrDurabilityUnknown and never as a refusal,
// that a client that does not decode its code gets only the bare durability
// unknown text, that the replaced Commit method fails, and that the server
// discarded the transaction and released the writer.
func TestTxResourceCommitRefusalRestoredByType(t *testing.T) {
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	cases := []struct {
		name string
		err  error
		// refused is false for an error that must arrive as an RPC error.
		refused bool
		is      func(error) bool
		// old commits as a client built before DURABILITY_UNKNOWN,
		// sending no known_error_code.
		old bool
	}{
		{"superseded", errors.Join(errors.New("head moved to seqno 3"), world.ErrSuperseded), true, world.IsSuperseded, false},
		{"stale generation", errors.Join(errors.New("lease at generation 4"), coord.ErrStaleGeneration), true, func(err error) bool {
			return errors.Is(err, coord.ErrStaleGeneration) && world.IsSuperseded(err)
		}, false},
		{"root not held", errors.Join(errors.New("set roots of bucket b"), block.ErrRootNotHeld), true, func(err error) bool {
			return errors.Is(err, block.ErrRootNotHeld) && !world.IsSuperseded(err)
		}, false},
		// An unknown outcome may be durable, so it is never a refusal, even
		// when its error also wraps a supersession.
		{"durability unknown", errors.Join(world.ErrDurabilityUnknown, world.ErrSuperseded), false, func(err error) bool {
			return errors.Is(err, world.ErrDurabilityUnknown) && !world.IsSuperseded(err)
		}, false},
		// A client that does not decode the code reads errors by their text,
		// so it gets the sentinel's text alone, mentioning no
		// supersession the client might read as a write to send again.
		{"durability unknown to an old client", errors.Join(world.ErrDurabilityUnknown, world.ErrSuperseded), false, func(err error) bool {
			return err.Error() == world.ErrDurabilityUnknown.Error()
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tx, err := tb.Engine.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			refused := &failedCommitTx{Tx: tx, err: c.err}
			resource := NewMount(nil, nil, nil).NewTxResource(refused, nil)
			defer resource.Release()
			rpc := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(resource.mux)))
			client := s4wave_world.NewSRPCTxResourceServiceClient(rpc)

			// A client of the method CommitTx replaced fails closed: it gets an
			// error, and the transaction is untouched.
			oldErr := rpc.ExecCall(ctx, s4wave_world.SRPCTxResourceServiceServiceID, "Commit", &s4wave_world.CommitRequest{}, &s4wave_world.CommitResponse{})
			if oldErr == nil || refused.discards != 0 {
				t.Fatalf("old Commit method = %v with %d discards, want an error and none", oldErr, refused.discards)
			}

			// A refusal arrives by type, reading as the server's
			// error. Any other failure arrives as an RPC error.
			if c.old {
				err = rpc.ExecCall(ctx, s4wave_world.SRPCTxResourceServiceServiceID, "CommitTx", &s4wave_world.CommitRequest{}, &s4wave_world.CommitResponse{})
			} else {
				err = s4wave_world.CommitRemote(ctx, client)
			}
			var refusal *s4wave_world.CommitRefusal
			if err == nil || errors.As(err, &refusal) != c.refused || !c.is(err) {
				t.Fatalf("commit = %v, want refused=%v for %s", err, c.refused, c.name)
			}
			if c.refused && err.Error() != c.err.Error() {
				t.Fatalf("commit message = %q, want %q", err.Error(), c.err.Error())
			}

			// The server discarded the transaction and released the writer: a
			// repeat CommitTx is refused as closed rather than retried on the
			// same transaction, and the next writer opens.
			if refused.discards != 1 {
				t.Fatalf("discards = %d, want 1", refused.discards)
			}
			if err := s4wave_world.CommitRemote(ctx, client); err == nil || errors.As(err, &refusal) {
				t.Fatalf("second commit = %v, want the closed transaction error", err)
			}
			nextCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			next, err := tb.Engine.NewTransaction(nextCtx, true)
			if err != nil {
				t.Fatalf("refused commit retained the writer: %v", err)
			}
			next.Discard()
		})
	}
}

func TestTxResourceFailedCommitReleasesWriter(t *testing.T) {

	// context ctx.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	tx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("commit failed")
	resource := NewMount(nil, nil, nil).NewTxResource(&failedCommitTx{Tx: tx, err: failure}, nil)
	defer resource.Release()
	if _, err := resource.CommitTx(ctx, &s4wave_world.CommitRequest{}); !errors.Is(err, failure) {
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

func TestUnavailableRefusal(t *testing.T) {
	// An unavailable plugin reads as a refusal only when it kept nothing: it
	// was never sent the operation, or the transaction it failed is discarded.
	for _, c := range []struct {
		sent, atomic, ok bool
	}{{false, false, true}, {false, true, true}, {true, true, true}, {true, false, false}} {
		err := &world.PluginUnavailableError{PluginID: "p", OperationTypeID: "op", Sent: c.sent, Err: errors.New("worker crashed")}
		code, _, ok := opErrorRefusal(err, c.atomic)
		if ok != c.ok || (ok && code != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_UNAVAILABLE) {
			t.Fatalf("sent %v atomic %v: refusal = %v %v, want ok %v", c.sent, c.atomic, code, ok, c.ok)
		}
	}
}

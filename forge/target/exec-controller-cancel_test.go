package forge_target

import (
	"context"
	"testing"
)

func TestExecCancelSignal(t *testing.T) {
	// Verify a context without execution custody has no cancellation signal.
	if signal := ExecCancelSignal(context.Background()); signal != nil {
		t.Fatal("cancel signal present without execution custody")
	}

	// Attach an open execution cancellation signal to the context.
	cancelCh := make(chan struct{})
	ctx := WithExecCancelSignal(context.Background(), cancelCh)

	// Verify the attached execution signal remains open before cancellation.
	select {
	case <-ExecCancelSignal(ctx):
		t.Fatal("cancel signal closed before cancellation")
	default:
	}

	// Cancel the execution through its attached signal.
	close(cancelCh)

	// Verify the execution context exposes the closed cancellation signal.
	select {
	case <-ExecCancelSignal(ctx):
	default:
		t.Fatal("cancel signal remained open after cancellation")
	}
}

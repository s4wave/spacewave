package forge_target

import (
	"context"
	"testing"
)

func TestExecCancelSignal(t *testing.T) {
	if signal := ExecCancelSignal(context.Background()); signal != nil {
		t.Fatal("cancel signal present without execution custody")
	}

	cancelCh := make(chan struct{})
	ctx := WithExecCancelSignal(context.Background(), cancelCh)
	select {
	case <-ExecCancelSignal(ctx):
		t.Fatal("cancel signal closed before cancellation")
	default:
	}
	close(cancelCh)
	select {
	case <-ExecCancelSignal(ctx):
	default:
		t.Fatal("cancel signal remained open after cancellation")
	}
}

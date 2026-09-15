package block

import (
	"testing"

	"github.com/pkg/errors"
)

// TestWriteAtRootWaitsForWorkersAfterEncodeError verifies that a failed write
// joins an encoder already running on an independent sibling subtree.
func TestWriteAtRootWaitsForWorkersAfterEncodeError(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	failed := make(chan struct{})
	done := make(chan error, 1)
	want := errors.New("transaction worker marshal failed")
	tx, root := NewTransaction(NopStoreOps{}, nil, nil, nil)
	root.SetBlock(&transactionWorkerBlock{}, true)
	root.FollowRef(1, nil).SetBlock(&transactionWorkerBlock{marshal: func() ([]byte, error) {
		close(started)
		<-release
		return nil, nil
	}}, true)
	root.FollowRef(2, nil).SetBlock(&transactionWorkerBlock{marshal: func() ([]byte, error) {
		<-started
		close(failed)
		return nil, want
	}}, true)
	go func() {
		_, _, err := tx.Write(t.Context(), true)
		done <- err
	}()
	<-failed
	select {
	case err := <-done:
		close(release)
		t.Fatalf("write returned while sibling encoder was running: %v", err)
	default:
	}
	close(release)
	if err := <-done; !errors.Is(err, want) {
		t.Fatalf("write error=%v want=%v", err, want)
	}
}

// transactionWorkerBlock exposes controlled encoder lifetimes without I/O.
type transactionWorkerBlock struct {
	// marshal supplies this block's encode behavior when non-nil.
	marshal func() ([]byte, error)
}

// MarshalBlock delegates to the controlled encoder, or emits an empty root.
func (b *transactionWorkerBlock) MarshalBlock() ([]byte, error) {
	if b.marshal != nil {
		return b.marshal()
	}
	return nil, nil
}

// UnmarshalBlock accepts the empty test payload.
func (*transactionWorkerBlock) UnmarshalBlock([]byte) error { return nil }

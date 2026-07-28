package block

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/hash"
)

var errTransactionWorkerMarshal = errors.New("transaction worker marshal failed")

func TestWriteAtRootWaitsForWorkersAfterEncodeError(t *testing.T) {
	oldEncodeConcurrency := maxEncodeConcurrency
	maxEncodeConcurrency = 0
	defer func() {
		maxEncodeConcurrency = oldEncodeConcurrency
	}()

	marshalDone := make(chan struct{})
	tx, rootCursor := NewTransaction(transactionWorkerStore{}, nil, nil, nil)
	rootCursor.SetBlock(&transactionWorkerRoot{
		marshalDone: marshalDone,
	}, true)
	childCursor := rootCursor.FollowRef(1, nil)
	childCursor.SetBlock(&transactionWorkerErrorBlock{}, true)

	if _, _, err := tx.Write(context.Background(), true); !errors.Is(err, errTransactionWorkerMarshal) {
		t.Fatalf("expected marshal error, got %v", err)
	}
	select {
	case <-marshalDone:
	default:
		t.Error("Write returned before its encode workers stopped")
	}

	// Keep the test alive long enough for an incorrectly detached worker to
	// finish mutating the transaction, so the race detector observes the bug.
	<-marshalDone
	time.Sleep(10 * time.Millisecond)
}

type transactionWorkerRoot struct {
	ref         *BlockRef
	marshalDone chan struct{}
}

func (r *transactionWorkerRoot) MarshalBlock() ([]byte, error) {
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		runtime.Gosched()
	}
	close(r.marshalDone)
	return []byte{1}, nil
}

func (r *transactionWorkerRoot) UnmarshalBlock([]byte) error {
	return nil
}

func (r *transactionWorkerRoot) ApplyBlockRef(id uint32, ref *BlockRef) error {
	if id == 1 {
		r.ref = ref.Clone()
	}
	return nil
}

func (r *transactionWorkerRoot) GetBlockRefs() (map[uint32]*BlockRef, error) {
	return map[uint32]*BlockRef{1: r.ref}, nil
}

func (r *transactionWorkerRoot) GetBlockRefCtor(uint32) Ctor {
	return func() Block {
		return &transactionWorkerErrorBlock{}
	}
}

type transactionWorkerErrorBlock struct{}

func (*transactionWorkerErrorBlock) MarshalBlock() ([]byte, error) {
	return nil, errTransactionWorkerMarshal
}

func (*transactionWorkerErrorBlock) UnmarshalBlock([]byte) error {
	return nil
}

type transactionWorkerStore struct {
	NopStoreOps
}

func (transactionWorkerStore) GetHashType() hash.HashType {
	return DefaultHashType
}

func (transactionWorkerStore) PutBlock(_ context.Context, _ []byte, opts *PutOpts) (*BlockRef, bool, error) {
	return opts.GetForceBlockRef().Clone(), false, nil
}

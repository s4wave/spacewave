package kvtx

import (
	"context"
	"errors"
	"testing"
)

func TestTxStoreTxGetBatchValidatesEmptyKeyBeforeDiscarded(t *testing.T) {
	ctx := context.Background()
	tx, err := NewTxStoreTx(txStoreTxBatchNoopOps{})
	if err != nil {
		t.Fatal(err)
	}
	tx.Discard()

	_, _, err = tx.Get(ctx, nil)
	if !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("Get after discard with empty key err = %v, want %v", err, ErrEmptyKey)
	}
	_, _, err = tx.GetBatch(ctx, [][]byte{nil})
	if !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("GetBatch after discard with empty key err = %v, want %v", err, ErrEmptyKey)
	}
}

type txStoreTxBatchNoopOps struct{}

func (txStoreTxBatchNoopOps) Size(context.Context) (uint64, error) {
	return 0, nil
}

func (txStoreTxBatchNoopOps) Get(context.Context, []byte) ([]byte, bool, error) {
	return nil, false, nil
}

func (txStoreTxBatchNoopOps) Exists(context.Context, []byte) (bool, error) {
	return false, nil
}

func (txStoreTxBatchNoopOps) Set(context.Context, []byte, []byte) error {
	return ErrNotWrite
}

func (txStoreTxBatchNoopOps) Delete(context.Context, []byte) error {
	return ErrNotWrite
}

func (txStoreTxBatchNoopOps) ScanPrefix(context.Context, []byte, func([]byte, []byte) error) error {
	return nil
}

func (txStoreTxBatchNoopOps) ScanPrefixKeys(context.Context, []byte, func([]byte) error) error {
	return nil
}

func (txStoreTxBatchNoopOps) Iterate(context.Context, []byte, bool, bool) Iterator {
	return NewErrIterator(nil)
}

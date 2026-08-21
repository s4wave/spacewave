package hydra_git

import (
	"context"
	"sync/atomic"
)

// NewFuncTx constructs a Tx from a Storer plus commit/discard callbacks.
func NewFuncTx(
	storer Storer,
	commit func(ctx context.Context) error,
	discard func(),
) Tx {
	return &funcTx{
		Storer:    storer,
		commitFn:  commit,
		discardFn: discard,
	}
}

type funcTx struct {
	Storer

	commitFn  func(ctx context.Context) error
	discardFn func()
	discarded atomic.Bool
}

func (t *funcTx) Commit(ctx context.Context) error {
	if t == nil || t.commitFn == nil {
		return nil
	}
	return t.commitFn(ctx)
}

func (t *funcTx) Discard() {
	if t == nil {
		return
	}
	if t.discarded.CompareAndSwap(false, true) {
		if t.discardFn != nil {
			t.discardFn()
		}
	}
}

// _ is a type assertion
var _ Tx = (*funcTx)(nil)

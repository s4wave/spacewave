package store_kvtx_vlogger

import (
	"context"

	kvtx_vlogger "github.com/s4wave/spacewave/db/kvtx/vlogger"
	hydra_store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/sirupsen/logrus"
)

// VLoggerStore wraps a KVTx store to verbosely log all operations.
type VLoggerStore struct {
	*kvtx_vlogger.VLoggerStore
	store hydra_store_kvtx.Store
}

func NewVLogger(le *logrus.Entry, store hydra_store_kvtx.Store) *VLoggerStore {
	vstore := kvtx_vlogger.NewVLogger(le, store)
	return &VLoggerStore{VLoggerStore: vstore, store: store}
}

// Unwrap returns the underlying store.
func (v *VLoggerStore) Unwrap() hydra_store_kvtx.Store {
	return v.store
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (v *VLoggerStore) Execute(ctx context.Context) error {
	return v.store.Execute(ctx)
}

// _ is a type assertion
var _ hydra_store_kvtx.Store = ((*VLoggerStore)(nil))

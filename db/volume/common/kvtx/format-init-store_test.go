package kvtx

import (
	"context"

	db_kvtx "github.com/s4wave/spacewave/db/kvtx"
)

// formatInitStore admits the second opener before the first takes its writer.
type formatInitStore struct {
	// Store supplies the real transactions.
	db_kvtx.Store
	// beforeWrite admits a competing opener once, before the first writer.
	beforeWrite func()
}

// NewTransaction invokes the competing initializer before acquiring the writer turn.
func (s *formatInitStore) NewTransaction(ctx context.Context, write bool) (db_kvtx.Tx, error) {
	// Admit the competing opener before the backing store takes its writer.
	if write && s.beforeWrite != nil {
		hook := s.beforeWrite
		s.beforeWrite = nil
		hook()
	}

	// Acquire the real transaction after the competing commits.
	return s.Store.NewTransaction(ctx, write)
}

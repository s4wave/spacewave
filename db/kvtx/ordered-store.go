package kvtx

import "context"

// OrderedStore is a store whose write transactions commit with write
// ordering: each commit becomes durable with the next durable commit of the
// store beneath it, without a flush of its own. Use it for state a crash may
// rewind to an earlier durable point, such as an index derived from other
// durable state.
type OrderedStore struct {
	// store is the store beneath.
	store Store
}

// NewOrderedStore wraps store so its write transactions commit ordered.
func NewOrderedStore(store Store) *OrderedStore {
	return &OrderedStore{store: store}
}

// NewTransaction returns a transaction whose Commit is ordered when write is
// set.
func (s *OrderedStore) NewTransaction(ctx context.Context, write bool) (Tx, error) {
	tx, err := s.store.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return WithOrderedCommit(tx), nil
}

// _ is a type assertion
var _ Store = (*OrderedStore)(nil)

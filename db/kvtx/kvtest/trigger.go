// Package kvtx_kvtest contains shared transaction behavior tests and fixtures.
//
// Matrix rule: use FaultStore for portable replay coverage. Add a backend-native
// stale trigger only when it proves a producer or runtime behavior that this
// seam cannot exercise.
package kvtx_kvtest

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/kvtx"
)

// FaultBoundary selects the transaction boundary at which a fault is injected.
type FaultBoundary uint8

const (
	// FaultBeforeCommit injects one invalid-snapshot error before delegating to
	// the wrapped transaction's Commit method.
	FaultBeforeCommit FaultBoundary = iota + 1
)

// FaultStore wraps a real store and injects one typed transaction failure.
// Every transaction is opened by the wrapped store. The transaction wrapper
// embeds the real transaction so operations outside the selected boundary are
// delegated unchanged. Its counters let tests reject helper-only mocks that
// skip backend opening or body execution.
type FaultStore struct {
	store    kvtx.Store
	boundary FaultBoundary

	mu                sync.Mutex
	injected          bool
	opened            int
	discarded         int
	discardedAttempts []int
	delegatedCommits  int
}

// NewFaultStore constructs a deterministic one-shot fault wrapper around store.
func NewFaultStore(store kvtx.Store, boundary FaultBoundary) *FaultStore {
	return &FaultStore{store: store, boundary: boundary}
}

// NewTransaction opens a transaction from the wrapped store and records the
// real attempt. The returned transaction delegates all unselected operations.
func (s *FaultStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.opened++
	attempt := s.opened
	s.mu.Unlock()

	return &faultTx{FaultStore: s, Tx: tx, attempt: attempt}, nil
}

// Opened reports how many transactions the wrapped store opened.
func (s *FaultStore) Opened() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opened
}

// Discarded reports how many wrapped transactions were discarded.
func (s *FaultStore) Discarded() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.discarded
}

// DiscardedAttempts reports the wrapped transaction attempts that were
// discarded, in discard order.
func (s *FaultStore) DiscardedAttempts() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.discardedAttempts...)
}

// DelegatedCommits reports successful calls delegated to the wrapped
// transaction's Commit method.
func (s *FaultStore) DelegatedCommits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.delegatedCommits
}

func (s *FaultStore) inject(boundary FaultBoundary) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.boundary != boundary || s.injected {
		return false
	}
	s.injected = true
	return true
}

func (s *FaultStore) recordDiscard(attempt int) {
	s.mu.Lock()
	s.discarded++
	s.discardedAttempts = append(s.discardedAttempts, attempt)
	s.mu.Unlock()
}

func (s *FaultStore) recordDelegatedCommit() {
	s.mu.Lock()
	s.delegatedCommits++
	s.mu.Unlock()
}

// faultTx delegates every operation except the selected fault boundary.
type faultTx struct {
	*FaultStore
	kvtx.Tx
	attempt int
}

func (t *faultTx) Commit(ctx context.Context) error {
	if t.inject(FaultBeforeCommit) {
		return kvtx.ErrInvalidSnapshot
	}
	err := t.Tx.Commit(ctx)
	if err == nil {
		t.recordDelegatedCommit()
	}
	return err
}

func (t *faultTx) Discard() {
	t.recordDiscard(t.attempt)
	t.Tx.Discard()
}

// Attempt identifies the wrapped transaction opened for this attempt.
func (t *faultTx) Attempt() int {
	return t.attempt
}

var _ kvtx.Store = (*FaultStore)(nil)
var _ kvtx.Tx = (*faultTx)(nil)

package session_controller

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

type scanConflictStore struct {
	kvtx.Store
	opened int
}

func (s *scanConflictStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	s.opened++
	return &scanConflictTx{Tx: tx, failScan: !write && s.opened == 1}, nil
}

type scanConflictTx struct {
	kvtx.Tx
	failScan bool
}

func (t *scanConflictTx) ScanPrefix(ctx context.Context, prefix []byte, cb func([]byte, []byte) error) error {
	if err := t.Tx.ScanPrefix(ctx, prefix, cb); err != nil {
		return err
	}
	if t.failScan {
		return kvtx.ErrInvalidSnapshot
	}
	return nil
}

func TestListSessionsRetriesGenerationChangeBetweenSizeAndScan(t *testing.T) {
	// Open an in-memory Session store for the scan failure scenario.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Persist the existing Session registration before injecting a transaction failure.
	entry := &session.SessionListEntry{SessionIndex: 7}
	data, err := entry.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Set(ctx, sessionListEntryKey(7), data); err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	writeTx.Discard()

	// List Sessions through a store that invalidates the first scan.
	conflicts := &scanConflictStore{Store: store}
	controller := &Controller{objStore: conflicts}
	entries, err := controller.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Session census retries and returns the stored registration.
	if conflicts.opened != 2 {
		t.Fatalf("opened transactions = %d, want 2", conflicts.opened)
	}
	if len(entries) != 1 || entries[0].GetSessionIndex() != 7 {
		t.Fatalf("entries = %v, want session 7", entries)
	}
}

type partialScanFailureStore struct {
	kvtx.Store
	err error
}

func (s *partialScanFailureStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &partialScanFailureTx{Tx: tx, err: s.err}, nil
}

type partialScanFailureTx struct {
	kvtx.Tx
	err error
}

func (t *partialScanFailureTx) ScanPrefix(ctx context.Context, prefix []byte, cb func([]byte, []byte) error) error {
	if err := t.Tx.ScanPrefix(ctx, prefix, cb); err != nil {
		return err
	}
	return t.err
}

func TestListSessionsReturnsNilAfterPartialScanFailure(t *testing.T) {
	// Open an in-memory Session store for the scan failure scenario.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Persist the existing Session registration before injecting a transaction failure.
	entry := &session.SessionListEntry{SessionIndex: 7}
	data, err := entry.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Set(ctx, sessionListEntryKey(7), data); err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	writeTx.Discard()

	// List Sessions through a store that fails after yielding its entries.
	scanErr := errors.New("scan failed")
	controller := &Controller{objStore: &partialScanFailureStore{Store: store, err: scanErr}}
	entries, err := controller.ListSessions(ctx)

	// Verify the Session census returns the scan failure without partial entries.
	if !errors.Is(err, scanErr) {
		t.Fatalf("error = %v, want %v", err, scanErr)
	}
	if entries != nil {
		t.Fatalf("entries = %v, want nil", entries)
	}
}

// registerConflictStore invalidates the first write transaction's census scan.
type registerConflictStore struct {
	kvtx.Store
	opened int
}

func (s *registerConflictStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	s.opened++
	return &scanConflictTx{Tx: tx, failScan: write && s.opened == 1}, nil
}

type commitConflictStore struct {
	kvtx.Store
	opened int
}

func (s *commitConflictStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	s.opened++
	return &commitConflictTx{Tx: tx, failCommit: write && s.opened == 1}, nil
}

// commitConflictTx models a backend whose storage snapshot was invalidated by
// a foreign commit after the transaction body ran.
type commitConflictTx struct {
	kvtx.Tx
	failCommit bool
}

func (t *commitConflictTx) Commit(ctx context.Context) error {
	if t.failCommit {
		return kvtx.ErrInvalidSnapshot
	}
	return t.Tx.Commit(ctx)
}

func TestRegisterSessionRetriesGenerationChangeBetweenSizeAndScan(t *testing.T) {
	// Open an in-memory Session store for the scan failure scenario.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Persist the existing Session registration before injecting a transaction failure.
	existing := &session.SessionListEntry{
		SessionIndex: 7,
		SessionRef: &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
			Id: "existing",
		}},
	}
	data, err := existing.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Set(ctx, sessionListEntryKey(7), data); err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	writeTx.Discard()

	// Register a Session through a store that invalidates the first write scan.
	conflicts := &registerConflictStore{Store: store}
	controller := &Controller{objStore: conflicts}
	ref := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{Id: "new"}}
	registered, err := controller.RegisterSession(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify registration retries and allocates the next Session index.
	if conflicts.opened != 2 {
		t.Fatalf("opened transactions = %d, want 2", conflicts.opened)
	}
	if registered.GetSessionIndex() != 8 || !registered.GetSessionRef().EqualVT(ref) {
		t.Fatalf("registered = %v, want new session at index 8", registered)
	}

	// Verify the Session registry retains both the existing and new attachments.
	entries, err := controller.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %v, want existing and new sessions", entries)
	}
}

func TestUpdateSessionMetadataRetriesGenerationChangeDuringRefLookup(t *testing.T) {
	// Open an in-memory Session store for the scan failure scenario.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	ref := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
		Id: "existing",
	}}

	// Open the write transaction that seeds the existing Session reference.
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Persist the existing Session registration before injecting a transaction failure.
	existing := &session.SessionListEntry{SessionIndex: 7, SessionRef: ref}
	data, err := existing.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Set(ctx, sessionListEntryKey(7), data); err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	writeTx.Discard()

	// Update Session metadata through a store that invalidates the first commit.
	conflicts := &commitConflictStore{Store: store}
	controller := &Controller{objStore: conflicts}
	const createdAt = int64(1700000000000)
	err = controller.UpdateSessionMetadata(ctx, ref, &session.SessionMetadata{
		ProviderDisplayName: "Local",
		CreatedAt:           createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the metadata update retries its invalidated transaction.
	if conflicts.opened != 2 {
		t.Fatalf("opened transactions = %d, want 2", conflicts.opened)
	}

	// Verify the Session metadata retains the requested creation time.
	stored, err := controller.GetSessionMetadata(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.GetCreatedAt() != createdAt {
		t.Fatalf("stored = %v, want metadata with created_at %d", stored, createdAt)
	}

	// A ref with no session entry is a silent no-op.
	missing := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
		Id: "absent",
	}}
	if err := controller.UpdateSessionMetadata(ctx, missing, &session.SessionMetadata{}); err != nil {
		t.Fatal(err)
	}
	stored, err = controller.GetSessionMetadata(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.GetCreatedAt() != createdAt {
		t.Fatalf("stored = %v, want unchanged metadata", stored)
	}
}

func TestDeleteSessionRetriesGenerationChangeDuringScan(t *testing.T) {
	// Open an in-memory Session store for the scan failure scenario.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	ref := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
		Id: "existing",
	}}

	// Open the write transaction that seeds the Session to delete.
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Persist the existing Session registration before injecting a transaction failure.
	existing := &session.SessionListEntry{SessionIndex: 7, SessionRef: ref}
	data, err := existing.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Set(ctx, sessionListEntryKey(7), data); err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	writeTx.Discard()

	// Delete the Session through a store that invalidates the first commit.
	conflicts := &commitConflictStore{Store: store}
	controller := &Controller{objStore: conflicts}
	err = controller.DeleteSession(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Session deletion retries its invalidated transaction.
	if conflicts.opened != 2 {
		t.Fatalf("opened transactions = %d, want 2", conflicts.opened)
	}

	// Verify the Session registry contains no attachments after deletion.
	entries, err := controller.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %v, want none after delete", entries)
	}

	// A ref with no session entry is a silent no-op.
	absent := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
		Id: "absent",
	}}
	if err := controller.DeleteSession(ctx, absent); err != nil {
		t.Fatal(err)
	}
}

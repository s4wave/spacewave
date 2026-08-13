package session_controller

import (
	"context"
	"errors"
	"testing"

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
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
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

	conflicts := &scanConflictStore{Store: store}
	controller := &Controller{objStore: conflicts}
	entries, err := controller.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
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
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
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

	scanErr := errors.New("scan failed")
	controller := &Controller{objStore: &partialScanFailureStore{Store: store, err: scanErr}}
	entries, err := controller.ListSessions(ctx)
	if !errors.Is(err, scanErr) {
		t.Fatalf("error = %v, want %v", err, scanErr)
	}
	if entries != nil {
		t.Fatalf("entries = %v, want nil", entries)
	}
}

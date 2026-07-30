package world_block

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

func TestCoordinatedWriteSnapshotClassificationIsTyped(t *testing.T) {
	if !isCoordinatedWriteSnapshotError(errors.Join(errors.New("backend detail"), kvtx.ErrInvalidSnapshot)) {
		t.Fatal("typed invalid snapshot was not classified")
	}
	if isCoordinatedWriteSnapshotError(block.ErrNotFound) {
		t.Fatal("ordinary missing block was classified as a snapshot")
	}
	if isCoordinatedWriteSnapshotError(errors.New("panic: page 2 already freed")) {
		t.Fatal("diagnostic text was classified as a snapshot")
	}
}

type replayCommitStore struct {
	kvtx.Store
	faultStore *kvtest.FaultStore
}

func (s *replayCommitStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write && s.faultStore != nil && s.faultStore.DelegatedCommits() == 0 {
		return s.faultStore.NewTransaction(ctx, true)
	}
	return s.Store.NewTransaction(ctx, write)
}

func TestEngineCreateObjectCommitReplaysIdentically(t *testing.T) {
	type result struct {
		objectRef *bucket.ObjectRef
		revision  uint64
		blockData map[string][]byte
	}
	run := func(t *testing.T, injectFault bool) (result, *kvtest.FaultStore) {
		t.Helper()
		ctx := t.Context()
		backend := store_kvtx_inmem.NewStore()
		commitStore := &replayCommitStore{Store: backend}
		blockStore := block_store_kvtx.NewKVTxBlock(
			store_kvkey.NewDefaultKVKey(),
			commitStore,
			0,
			false,
		)
		blockTx, blockCursor := block.NewTransaction(blockStore, nil, nil, nil)
		state, err := NewWorldState(
			ctx,
			logrus.NewEntry(logrus.New()),
			true,
			blockTx,
			blockCursor,
			blockStore,
			nil,
			nil,
			nil,
			world_mock.LookupMockOp,
			false,
		)
		if err != nil {
			t.Fatal(err)
		}
		engineTx := &EngineTx{writeTx: NewTx(state)}
		if _, err := engineTx.CreateObject(
			ctx,
			"replay/object",
			&bucket.ObjectRef{BucketId: "replay-bucket"},
		); err != nil {
			t.Fatal(err)
		}

		var faultStore *kvtest.FaultStore
		if injectFault {
			faultStore = kvtest.NewFaultStore(backend, kvtest.FaultBeforeCommit)
			commitStore.faultStore = faultStore
		}
		if _, err := engineTx.writeTx.CommitBlockTransaction(ctx); err != nil {
			t.Fatal(err)
		}
		commitStore.faultStore = nil

		object, found, err := state.GetObject(ctx, "replay/object")
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatal("committed object was not found")
		}
		objectRef, revision, err := object.GetRootRef(ctx)
		if err != nil {
			t.Fatal(err)
		}

		tx, err := backend.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		data := make(map[string][]byte)
		if err := tx.ScanPrefix(ctx, nil, func(key, value []byte) error {
			data[string(key)] = bytes.Clone(value)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return result{
			objectRef: objectRef,
			revision:  revision,
			blockData: data,
		}, faultStore
	}

	want, _ := run(t, false)
	got, faultStore := run(t, true)
	if !got.objectRef.EqualVT(want.objectRef) {
		t.Fatalf("object ref = %v, want %v", got.objectRef, want.objectRef)
	}
	if got.revision != want.revision {
		t.Fatalf("object revision = %d, want %d", got.revision, want.revision)
	}
	if len(got.blockData) != len(want.blockData) {
		t.Fatalf(
			"committed block count = %d, want %d",
			len(got.blockData),
			len(want.blockData),
		)
	}
	if got := faultStore.Opened(); got != 2 {
		t.Fatalf("opened transactions = %d, want 2", got)
	}
	if got := faultStore.DelegatedCommits(); got != 1 {
		t.Fatalf("delegated commits = %d, want 1", got)
	}
}

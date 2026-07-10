package kvtx_block

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	kvtx_txcache "github.com/s4wave/spacewave/db/kvtx/txcache"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestSimple is a basic tree test for all known implementations.
func TestSimple(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	testImpls := []KVImplType{
		KVImplType_KV_IMPL_TYPE_IAVL,
		KVImplType_KV_IMPL_TYPE_OKRA,
	}

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// construct a basic transform config.
	tconf, err := block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	for _, impl := range testImpls {
		oc, _, err := bucket_lookup.BuildEmptyCursor(
			ctx,
			tb.Bus,
			tb.Logger,
			tb.StepFactorySet,
			tb.BucketId,
			volID,
			tconf,
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}

		btx, bcs := oc.BuildTransaction(nil)
		kvs := NewKeyValueStore(impl)
		bcs.SetBlock(kvs, true)
		_, bcs, err = btx.Write(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}

		// buildTx builds a transaction which buffers changes in memory
		/*
			// buffer operations in memory before commit to block graph
			kvtx_txcache.NewTxWithCbs(ktx, write, nil, func() (kvtx.Tx, error) {
				return ktx, nil
			})
		*/
		buildStore := func(write bool) (kvtx.Store, kvtx.Tx) {
			ktx, err := BuildKvTransaction(ctx, bcs, write)
			if err != nil {
				t.Fatal(err.Error())
			}
			return kvtx_txcache.NewTxStore(ktx, write), ktx
		}

		store, storeTx := buildStore(true)
		err = kvtx_kvtest.TestAll(ctx, store)
		if err != nil {
			t.Fatal(err.Error())
		}
		err = storeTx.Commit(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		store, storeTx = buildStore(false)
		ktx, err := store.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		_, found, err := ktx.Get(ctx, []byte("ba"))
		if err != nil {
			t.Fatal(err.Error())
		}
		if !found {
			t.Fatalf("%s: expected ba to exist after commit", impl.String())
		}
		storeTx.Discard()

		t.Logf("successfully tested %s", impl.String())
	}
}

// TestSelectorSeed verifies that the root selector recognizes explicit IAVL and
// Okra roots without changing the zero/default implementation.
func TestSelectorSeed(t *testing.T) {
	ctx := context.Background()

	if got := NewKeyValueStore(0).GetImplType(); got != DefaultKeyValueStoreImpl {
		t.Fatalf("default impl = %s, want %s", got, DefaultKeyValueStoreImpl)
	}
	if got := NewKeyValueStoreForWorkload(WorkloadClassDefault).GetImplType(); got != DefaultKeyValueStoreImpl {
		t.Fatalf("workload default impl = %s, want %s", got, DefaultKeyValueStoreImpl)
	}

	for _, impl := range []KVImplType{KVImplType_KV_IMPL_TYPE_IAVL, KVImplType_KV_IMPL_TYPE_OKRA} {
		t.Run(impl.String(), func(t *testing.T) {
			_, bcs := block.NewTransaction(nil, nil, nil, nil)
			bcs.SetBlock(NewKeyValueStore(impl), true)

			kvs, err := LoadKeyValueStore(ctx, bcs)
			if err != nil {
				t.Fatal(err)
			}
			if err := kvs.Validate(); err != nil {
				t.Fatal(err)
			}

			ktx, err := BuildKvTransaction(ctx, bcs, false)
			if err != nil {
				t.Fatal(err)
			}
			defer ktx.Discard()

			size, err := ktx.Size(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if size != 0 {
				t.Fatalf("empty %s size = %d, want 0", impl, size)
			}
			exists, err := ktx.Exists(ctx, []byte("missing"))
			if err != nil {
				t.Fatal(err)
			}
			if exists {
				t.Fatalf("empty %s reported missing key as present", impl)
			}
			if _, err := ktx.GetCursorAtKey(ctx, []byte{}); err != kvtx.ErrEmptyKey {
				t.Fatalf("empty-key cursor lookup err = %v, want %v", err, kvtx.ErrEmptyKey)
			}

			if got := kvs.GetImplType(); got != impl {
				t.Fatalf("impl changed after write: %s != %s", got, impl)
			}
		})
	}
}

func TestBackendPolicyClassifiesWorkloads(t *testing.T) {
	for _, tc := range []struct {
		name     string
		workload WorkloadClass
		want     KVImplType
	}{
		{name: "default", workload: WorkloadClassDefault, want: KVImplType_KV_IMPL_TYPE_IAVL},
		{name: "tiny-metadata", workload: WorkloadClassTinyMetadata, want: KVImplType_KV_IMPL_TYPE_IAVL},
		{name: "graph-prefix-read", workload: WorkloadClassGraphPrefixRead, want: KVImplType_KV_IMPL_TYPE_OKRA},
		{name: "indexed-log", workload: WorkloadClassIndexedLog, want: KVImplType_KV_IMPL_TYPE_IAVL},
		{name: "cursor-value-read", workload: WorkloadClassCursorValueRead, want: KVImplType_KV_IMPL_TYPE_IAVL},
		{name: "write-churn", workload: WorkloadClassWriteChurn, want: KVImplType_KV_IMPL_TYPE_IAVL},
		{name: "gc-refgraph", workload: WorkloadClassGCRefGraph, want: KVImplType_KV_IMPL_TYPE_OKRA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := DefaultKeyValueStoreImplForWorkload(tc.workload); got != tc.want {
				t.Fatalf("policy impl = %s, want %s", got, tc.want)
			}
			if got := NewKeyValueStoreForWorkload(tc.workload).GetImplType(); got != tc.want {
				t.Fatalf("new store impl = %s, want %s", got, tc.want)
			}
		})
	}

	_, bcs := block.NewTransaction(nil, nil, nil, nil)
	bcs.SetBlock(&KeyValueStore{}, true)
	kvs, err := LoadKeyValueStore(context.Background(), bcs)
	if err != nil {
		t.Fatal(err)
	}
	if got := kvs.GetImplType(); got != LegacyKeyValueStoreImpl {
		t.Fatalf("legacy zero impl = %s, want %s", got, LegacyKeyValueStoreImpl)
	}
}

// TestSelectorSubBlocks verifies that each implementation owns only its root
// sub-block under the common KeyValueStore block.
func TestSelectorSubBlocks(t *testing.T) {
	iavlStore := NewKeyValueStore(KVImplType_KV_IMPL_TYPE_IAVL)
	iavlCtor := iavlStore.GetSubBlockCtor(2)
	if iavlCtor == nil {
		t.Fatal("missing IAVL root constructor")
	}
	if _, ok := iavlCtor(true).(*iavl.Node); !ok {
		t.Fatal("IAVL root constructor returned unexpected type")
	}
	if got := iavlStore.GetSubBlocks(); got[2] == nil || got[3] != nil {
		t.Fatalf("IAVL sub-block map = %#v", got)
	}

	okraStore := NewKeyValueStore(KVImplType_KV_IMPL_TYPE_OKRA)
	okraCtor := okraStore.GetSubBlockCtor(3)
	if okraCtor == nil {
		t.Fatal("missing Okra root constructor")
	}
	if _, ok := okraCtor(true).(*okra.Root); !ok {
		t.Fatal("Okra root constructor returned unexpected type")
	}
	if got := okraStore.GetSubBlocks(); got[2] != nil || got[3] == nil {
		t.Fatalf("Okra sub-block map = %#v", got)
	}

	if err := okraStore.ApplySubBlock(3, &okra.Root{}); err != nil {
		t.Fatal(err)
	}
	if err := okraStore.ApplySubBlock(3, &iavl.Node{}); err != block.ErrUnexpectedType {
		t.Fatalf("wrong Okra sub-block type err = %v, want %v", err, block.ErrUnexpectedType)
	}
}

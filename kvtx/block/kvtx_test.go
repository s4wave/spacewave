package kvtx_block

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	transform_snappy "github.com/aperturerobotics/hydra/block/transform/snappy"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_kvtest "github.com/aperturerobotics/hydra/kvtx/kvtest"
	kvtx_txcache "github.com/aperturerobotics/hydra/kvtx/txcache"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestSimple is a basic tree test for all known implementations.
func TestSimple(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	testImpls := []KVImplType{KVImplType_KV_IMPL_TYPE_IAVL}

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// construct a basic transform config.
	tconf, err := block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_snappy.Config{},
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
		_, bcs, err = btx.Write(true)
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
		_, found, err := ktx.Get(ctx, []byte("test-1"))
		if err != nil {
			t.Fatal(err.Error())
		}
		if !found {
			t.Fail()
		}
		storeTx.Discard()

		t.Logf("successfully tested %s", impl.String())
	}
}

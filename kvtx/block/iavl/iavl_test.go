package kvtx_block_iavl

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestSimple is a basic iavl tree test.
func TestSimple(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

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
		&transform_s2.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

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

	tr := NewAVLTree(oc)

	btx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	ilen, err := btx.Size(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ilen != 0 {
		t.FailNow()
	}

	key := []byte("test")
	h, err := btx.Exists(ctx, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, ok, err := btx.Get(ctx, key); ok || err != nil || h {
		t.FailNow()
	}

	val := []byte("tvalue")
	err = btx.Set(ctx, key, val)
	if err != nil {
		t.Fatal(err.Error())
	}

	ival, ok, err := btx.Get(ctx, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok || !bytes.Equal(ival, val) {
		t.FailNow()
	}

	/*
		n, err := tr.Len()
		if err != nil {
			t.Fatal(err.Error())
		}
		if n != 1 {
			t.FailNow()
		}
	*/

	/*
		rnRef := bt.GetRootNodeRef()
		bt = nil
		oc.Release()
		ncursor, err := object.BuildCursor(
			ctx,
			tb.Bus,
			tb.Logger,
			tb.StepFactorySet,
			volID,
			rnRef,
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}

		bt, err = LoadBTree(ncursor)
		if err != nil {
			t.Fatal(err.Error())
		}
		l, err := bt.Len()
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("loaded btree successfully w/ %d keys", l)

		oref, found, err := bt.Get(key)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("executed get(%s): found(%v) ref(%v)", key, found, oref)
		if !found || oref != nil {
			t.FailNow()
		}
		if _, err := bt.ReplaceOrInsert("test-2", nil); err != nil {
			t.Fatal(err.Error())
		}
		var keys []string
		err = bt.Ascend(func(key string) (ctnu bool, err error) {
			keys = append(keys, key)
			return true, nil
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		if len(keys) != 2 {
			t.Fatal("expected 2 keys from ascend")
		}
		t.Logf("ascend() returned keys: %v", keys)
		keys = nil
		err = bt.DescendLessOrEqual("test-", func(key string) (ctnu bool, err error) {
			keys = append(keys, key)
			return true, nil
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("descendLessOrEqual(test-) returned %v", keys)
		if keys[0] != "test" || len(keys) != 1 {
			t.Fail()
		}
		t.Logf("deleting key %s", key)
		if _, found, err := bt.Delete(key); err != nil || !found {
			t.Fatal(err.Error())
			t.FailNow()
		}
		t.Logf("deleting key %s", "test-2")
		if _, found, err := bt.Delete("test-2"); err != nil || !found {
			t.Fatal(err.Error())
			t.FailNow()
		}
		l, err = bt.Len()
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("key count after: %d", l)
		if l != 0 {
			t.FailNow()
		}
	*/
}

// TestIavl is a more comprehensive test.
func TestIavl(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)

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

	tr := NewAVLTree(oc)
	btx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	ilen, err := btx.Size(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ilen != 0 {
		t.FailNow()
	}

	key := []byte("test")
	h, err := btx.Exists(ctx, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, ok, err := btx.Get(ctx, key); ok || err != nil || h {
		t.FailNow()
	}

	kn := 5
	t.Logf("placing %d keys", kn)
	for i := 0; i < kn; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		val := []byte(fmt.Sprintf("key-%d", kn-i))

		err := btx.Set(ctx, key, val)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Log(string(key))
	}

	checkAll := func() {
		for i := kn - 1; i >= 0; i-- {
			key := []byte(fmt.Sprintf("key-%d", i))
			ival, ok, err := btx.Get(ctx, key)
			if err != nil {
				t.Fatal(err.Error())
			}
			if !ok || len(ival) == 0 {
				t.Fatalf("key not found %s", key)
			}
		}
	}

	checkAll()
	if err := btx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	btx, err = tr.NewAVLTreeTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	checkAll()
	keyCount := 0
	err = btx.ScanPrefix(ctx, []byte("key-"), func(key, val []byte) error {
		if len(key) == 0 || len(val) == 0 {
			t.FailNow()
		}
		keyCount++
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if keyCount != kn {
		t.Fatalf("counted %d keys expected %d", keyCount, kn)
	}

	btx.Discard()
	btx, err = tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	checkAll()
	for i := 0; i < kn; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		if i%2 == 0 {
			t.Logf("deleting key %s", key)
			err := btx.Delete(ctx, key)
			if err != nil {
				t.Fatal(err.Error())
			}
			_, bfound, err := btx.Get(ctx, key)
			if err != nil {
				t.Fatal(err.Error())
			}
			if bfound {
				t.Fatalf("key %s found after deleted", key)
			}
		}
	}

	if err := btx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	rref := tr.GetRootNodeRef()
	fc, err := oc.FollowRef(ctx, rref)
	if err != nil {
		t.Fatal(err.Error())
	}
	ft := NewAVLTree(fc)
	btx, err = ft.NewAVLTreeTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	expectedSize := kn / 2
	ns, err := btx.Size(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	trs := int(ns)
	if trs != expectedSize {
		t.Fatalf("removal size mismatch %d != expected %d", trs, expectedSize)
	}
	actLen := 0
	for i := 0; i < kn; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		keep := i%2 != 0
		_, exists, err := btx.Get(ctx, key)
		if err != nil {
			t.Fatal(err.Error())
		}
		if exists != keep {
			t.Fatalf("key %s exists %v (expected %v)", key, exists, keep)
		}
		if exists {
			actLen++
		}
	}
	if actLen != trs {
		t.Fatalf("length reported %d != actual length %d", trs, actLen)
	}

	btx.Discard()
}

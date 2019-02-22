package iavl

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block/object"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	transform_snappy "github.com/aperturerobotics/hydra/block/transform/snappy"
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
		&transform_snappy.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	oc, _, err := object.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		testbed.BucketId,
		volID,
		tconf,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	tr := NewAVLTree(oc)

	ilen := tr.Size()
	if ilen != 0 {
		t.FailNow()
	}

	key := "test"
	h, err := tr.Has(key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, ok, err := tr.Get(key); ok || err != nil || h {
		t.FailNow()
	}

	val := []byte("tvalue")
	iv, err := tr.Set(key, val)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !iv {
		t.FailNow()
	}

	ival, ok, err := tr.Get(key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok || bytes.Compare(ival, val) != 0 {
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

// TestStress is a basic iavl tree stress test.
func TestStress(t *testing.T) {
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

	oc, _, err := object.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		testbed.BucketId,
		volID,
		tconf,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	tr := NewAVLTree(oc)

	ilen := tr.Size()
	if ilen != 0 {
		t.FailNow()
	}

	key := "test"
	h, err := tr.Has(key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, ok, err := tr.Get(key); ok || err != nil || h {
		t.FailNow()
	}

	kn := 1000
	for i := 0; i < kn; i++ {
		key := fmt.Sprintf("key-%d", i)
		val := []byte(fmt.Sprintf("key-%d", kn-i))

		iv, err := tr.Set(key, val)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !iv {
			t.FailNow()
		}
	}

	for i := kn - 1; i >= 0; i-- {
		key := fmt.Sprintf("key-%d", i)
		ival, ok, err := tr.Get(key)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !ok || len(ival) == 0 {
			t.Fatalf("key not found %s", key)
		}
	}

	for i := 0; i < kn; i++ {
		key := fmt.Sprintf("key-%d", i)
		if i%2 == 0 {
			_, found, err := tr.Remove(key)
			if err != nil {
				t.Fatal(err.Error())
			}
			if !found {
				t.Fatalf("key not found %s", key)
			}
		}
	}

	expectedSize := kn / 2
	if trs := tr.Size(); int(trs) != expectedSize {
		t.Fatalf("removal size mismatch %d != expected %d", trs, expectedSize)
	}
}

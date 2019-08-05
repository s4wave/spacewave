package btree

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

// TestBTreeSimple tests simple btree functionality.
func TestBTreeSimple(t *testing.T) {
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

	bt := NewBTree(oc)
	tx, err := bt.NewBTreeTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}

	ilen, err := tx.Len()
	if err != nil {
		t.Fatal(err.Error())
	}
	if ilen != 0 {
		t.FailNow()
	}
	tx.Discard()

	tx, err = bt.NewBTreeTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	key := []byte("test")
	val := []byte("val1")
	val2 := []byte("val2")
	iv, err := tx.ReplaceOrInsert(key, val)
	if err != nil {
		t.Fatal(err.Error())
	}
	if iv != nil {
		t.FailNow()
	}

	ivb, ivbOk, err := tx.Get(key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ivbOk {
		t.FailNow()
	}
	if len(ivb) == 0 {
		t.FailNow()
	}

	iv, err = tx.ReplaceOrInsert(key, val2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(iv) != len(val) {
		t.FailNow()
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	tx, err = bt.NewBTreeTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}

	n, err := tx.Len()
	if err != nil {
		t.Fatal(err.Error())
	}
	if n != 1 {
		t.FailNow()
	}

	ivb, ivbOk, err = tx.Get(key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ivbOk {
		t.FailNow()
	}
	if !bytes.Equal(ivb, val2) {
		t.FailNow()
	}
	tx.Discard()

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

	bt = NewBTree(ncursor)
	tx, err = bt.NewBTreeTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}
	l, err := tx.Len()
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("loaded btree successfully w/ %d keys", l)

	oref, found, err := tx.Get(key)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("executed get(%s): found(%v) ref(%v)", key, found, oref)
	if !found || len(oref) == 0 {
		t.FailNow()
	}
	tx.Discard()

	tx, err = bt.NewBTreeTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := tx.ReplaceOrInsert([]byte("test-2"), nil); err != nil {
		t.Fatal(err.Error())
	}
	var keys []string
	err = tx.Ascend(func(key []byte) (ctnu bool, err error) {
		keys = append(keys, string(key))
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
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	tx, err = bt.NewBTreeTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = tx.DescendLessOrEqual([]byte("test-"), func(key []byte) (ctnu bool, err error) {
		keys = append(keys, string(key))
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
	if _, found, err := tx.Remove(key); err != nil || !found {
		t.Fatal(err.Error())
		t.FailNow()
	}
	t.Logf("deleting key %s", "test-2")
	if _, found, err := tx.Remove([]byte("test-2")); err != nil || !found {
		t.Fatal(err.Error())
		t.FailNow()
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	tx, err = bt.NewBTreeTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}
	l, err = tx.Len()
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("key count after: %d", l)
	if l != 0 {
		t.FailNow()
	}
	tx.Discard()
}

// TestBTreeStress stress tests btree functionality
func TestBTreeStress(t *testing.T) {
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

	bt := NewBTree(oc)
	tx, err := bt.NewBTreeTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}

	ilen, err := tx.Len()
	if err != nil {
		t.Fatal(err.Error())
	}
	if ilen != 0 {
		t.FailNow()
	}
	tx.Discard()

	tx, err = bt.NewBTreeTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	kn := 60
	for i := 0; i < kn; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		val := []byte(fmt.Sprintf("key-%d", kn-i))
		iv, err := tx.ReplaceOrInsert(key, val)
		if err != nil {
			t.Fatal(err.Error())
		}
		if iv != nil {
			t.FailNow()
		}
		t.Logf("set %s", string(key))
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	t.Log("committed")

	tx, err = bt.NewBTreeTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}

	for i := kn - 1; i >= 0; i-- {
		key := []byte(fmt.Sprintf("key-%d", i))
		ivb, ivbOk, err := tx.Get(key)
		if err != nil {
			t.Log(err.Error())
			t.Fail()
		}
		if !ivbOk {
			t.Logf("key not found: %s", string(key))
			t.Fail()
		} else if len(ivb) == 0 {
			t.Fail()
		} else {
			t.Logf("ok %s", string(key))
		}
	}

	/*
		n, err := tx.Len()
		if err != nil {
			t.Fatal(err.Error())
		}
		if n != 1 {
			t.FailNow()
		}

		ivb, ivbOk, err = tx.Get(key)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !ivbOk {
			t.FailNow()
		}
		if !bytes.Equal(ivb, val2) {
			t.FailNow()
		}
	*/
	tx.Discard()
}

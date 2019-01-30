package btree

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block/object"
	"github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/block/transform/chksum"
	"github.com/aperturerobotics/hydra/block/transform/snappy"
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

	bt, err := NewBTree(oc, 0)
	if err != nil {
		t.Fatal(err.Error())
	}

	ilen, err := bt.Len()
	if err != nil {
		t.Fatal(err.Error())
	}
	if ilen != 0 {
		t.FailNow()
	}

	key := "test"
	val := ((*object.ObjectRef)(nil))
	iv, err := bt.ReplaceOrInsert(key, val)
	if err != nil {
		t.Fatal(err.Error())
	}
	if iv != nil {
		t.FailNow()
	}

	iv, err = bt.ReplaceOrInsert(key, val)
	if err != nil {
		t.Fatal(err.Error())
	}
	if iv != nil {
		t.FailNow()
	}

	n, err := bt.Len()
	if err != nil {
		t.Fatal(err.Error())
	}
	if n != 1 {
		t.FailNow()
	}

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
}

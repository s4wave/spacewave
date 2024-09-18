package volume_test_test

import (
	"bytes"
	"context"
	"testing"

	kvtx_kvtest "github.com/aperturerobotics/hydra/kvtx/kvtest"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/sirupsen/logrus"
)

// TestBusObjectStore tests the bus backed object store.
func TestBusObjectStore(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	storeID := "test-store"
	storeVolume := tb.Volume.GetID()
	st := volume.NewBusObjectStore(ctx, tb.Bus, false, storeID, storeVolume)
	if err := kvtx_kvtest.TestAll(ctx, st); err != nil {
		t.Fatal(err.Error())
	}
}

// TestBuildObjectStoreAPI tests the build object store api directive.
func TestBuildObjectStoreAPI(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	storeID := "test-store"
	storeVolume := tb.Volume.GetID()
	val, _, ref, err := volume.ExBuildObjectStoreAPI(ctx, tb.Bus, false, storeID, storeVolume, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ref.Release()

	st := val.GetObjectStore()
	tx, err := st.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = tx.Set(ctx, []byte("test"), []byte("test-value"))
	if err != nil {
		t.Fatal(err.Error())
	}
	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	tx, err = st.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	tval, found, err := tx.Get(ctx, []byte("test"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fail()
	}
	if !bytes.Equal(tval, []byte("test-value")) {
		t.Fail()
	}
}

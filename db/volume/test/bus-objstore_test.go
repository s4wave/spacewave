package volume_test_test

import (
	"bytes"
	"context"
	"testing"

	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

// TestBusObjectStore tests the bus backed object store.
func TestBusObjectStore(t *testing.T) {
	// Prepare a test context and logger for the object store.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the Volume testbed that owns the bus-backed store.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Bind the object store to the testbed Volume and exercise its operations.
	storeID := "test-store"
	storeVolume := tb.Volume.GetID()
	st := volume.NewBusObjectStore(ctx, tb.Bus, false, storeID, storeVolume)
	if err := kvtx_kvtest.TestAll(ctx, st); err != nil {
		t.Fatal(err.Error())
	}
}

// TestBuildObjectStoreAPI tests the build object store api directive.
func TestBuildObjectStoreAPI(t *testing.T) {
	// Prepare a test context and logger for the build directive.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the Volume testbed for the object-store API directive.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Resolve the object store and retain its directive reference.
	storeID := "test-store"
	storeVolume := tb.Volume.GetID()
	val, _, ref, err := volume.ExBuildObjectStoreAPI(ctx, tb.Bus, false, storeID, storeVolume, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ref.Release()

	// Write a value through the resolved object store.
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

	// Open a read transaction to verify the committed value.
	tx, err = st.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read back the key and compare its stored bytes.
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

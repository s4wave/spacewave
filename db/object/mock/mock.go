package object_mock

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// BuildTestStore constructs a testbed-backed ObjectStore.
func BuildTestStore(t *testing.T) (object.ObjectStore, *testbed.Testbed) {
	// Prepare a logger and context for the in-memory testbed.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the testbed and release it during test cleanup.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Capture the test volume identity for logging.
	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// Mount an object store whose release the test owns.
	objs, objsRel, err := vol.AccessObjectStore(ctx, "test-store", func() {})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(objsRel)

	// Return the store and its owning testbed.
	return objs, tb
}

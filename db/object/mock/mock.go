package object_mock

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

func BuildTestStore(t *testing.T) (object.ObjectStore, *testbed.Testbed) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// pass a no-op released func
	objs, objsRel, err := vol.AccessObjectStore(ctx, "test-store", func() {})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(objsRel)

	return objs, tb
}

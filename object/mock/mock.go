package object_mock

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/object"
	"github.com/aperturerobotics/hydra/testbed"
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

	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	objs, err := vol.OpenObjectStore(ctx, "test-store")
	if err != nil {
		t.Fatal(err.Error())
	}
	return objs, tb
}

package testbed

import (
	"context"
	"testing"

	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	"github.com/sirupsen/logrus"
)

func TestTestbed(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := BuildTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// verify the world started ok
	eng := tb.GetWorldEngine()
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	tx.Discard()

	if _, err := bldr_manifest_world.CreateManifestStoreInEngine(ctx, eng, "testbed-plugin-host"); err != nil {
		t.Fatal(err.Error())
	}
}

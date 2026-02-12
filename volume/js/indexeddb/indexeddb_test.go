//go:build js

package volume_indexeddb

import (
	"context"
	"testing"

	volume_test "github.com/aperturerobotics/hydra/volume/test"
	"github.com/sirupsen/logrus"
)

// TestIndexedDB runs the basic volume test suite.
func TestIndexedDB(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	vol, err := NewIndexedDB(ctx, le, &Config{
		Verbose:      true,
		DatabaseName: "hydra/test/vol/idb",
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := volume_test.CheckVolume(ctx, vol); err != nil {
		t.Fatal(err.Error())
	}
}

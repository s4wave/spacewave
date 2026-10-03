package kvtx_block

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestStore tests the kvtx block store.
func TestStore(t *testing.T) {
	// Create a logger for the KV Store testbed.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start a testbed for KV Store operations.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty bucket cursor for the KV Store.
	bls, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Construct the KV Store with a root commit callback.
	st, err := NewStore(ctx, le, bls, func(nref *bucket.ObjectRef) error {
		le.Infof("root ref committed: %v", nref.MarshalString())
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Exercise the KV Store transaction contract.
	if err := kvtx_kvtest.TestAll(ctx, st); err != nil {
		t.Fatal(err.Error())
	}
}

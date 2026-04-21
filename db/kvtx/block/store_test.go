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
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	bls, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	st, err := NewStore(ctx, le, bls, func(nref *bucket.ObjectRef) error {
		le.Infof("root ref committed: %v", nref.MarshalString())
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	if err := kvtx_kvtest.TestAll(ctx, st); err != nil {
		t.Fatal(err.Error())
	}
}

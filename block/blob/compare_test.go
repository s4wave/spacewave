package blob

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/util/prng"
	"github.com/sirupsen/logrus"
)

// TestCompareBlobs tests comparing two large blobs.
func TestCompareBlobs(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	cs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cs.Release()

	// test not equal
	r1 := prng.BuildSeededReader([]byte("test-1"))
	r2 := prng.BuildSeededReader([]byte("test-2"))

	btx, bcs := cs.BuildTransactionAtRef(nil, nil)
	_, err = BuildBlob(ctx, int64(2048), r1, bcs, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, bcs, err = btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	btx2, bcs2 := cs.BuildTransactionAtRef(nil, nil)
	_, err = BuildBlob(ctx, int64(2048), r2, bcs2, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, bcs2, err = btx2.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	same, err := CompareBlobs(ctx, bcs, bcs2)
	if err == nil && same {
		err = errors.New("expected blobs to not be the same")
	}
	if err != nil {
		t.Fatal(err.Error())
	}
}

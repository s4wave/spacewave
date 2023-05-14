package blob

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestBuildBlobWithBytes tests building a Blob from a byte slice.
func TestBuildBlobWithBytes(t *testing.T) {
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

	data := []byte("hello world 1234")

	btx, bcs := cs.BuildTransactionAtRef(nil, nil)
	_, err = BuildBlobWithBytes(ctx, data, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}

	bref, _, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("blob written to %s", bref.MarshalString())

	cs.SetRootRef(bref)
	_, bcs = cs.BuildTransaction(nil)
	fetched, err := FetchToBytes(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(fetched, data) {
		t.Fatalf("mismatch of fetched data: %#v != expected %#v", fetched, data)
	}

	_, bcs = cs.BuildTransaction(nil)
	b1, err := UnmarshalBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := b1.ValidateFull(ctx, bcs); err != nil {
		t.Fatal(err.Error())
	}
}

package git_block

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/sirupsen/logrus"
)

// TestStorage_References runs a simple test of storing references.
func TestStorage_References(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	testbed.Verbose = true
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	btx, bcs := oc.BuildTransaction(nil)
	root := NewRepo()
	bcs.SetBlock(root, true)

	store, err := NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer store.Close()

	// check not exists
	_, err = store.Reference("notfound")
	if err != plumbing.ErrReferenceNotFound {
		t.Fail()
	}

	// store a reference
	testRef := plumbing.ReferenceName("main")
	err = store.SetReference(plumbing.NewSymbolicReference(testRef, "master"))
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Info("set reference 'main'")

	// check exists
	_, err = store.Reference(testRef)
	if err != nil {
		t.Fatal(err.Error())
	}

	// write
	rootRef, bcs, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("wrote new root node %s", rootRef.MarshalString())

	store, err = NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// check exists
	ref, err := store.Reference(testRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ref.Name() != testRef {
		t.Fail()
	}
}

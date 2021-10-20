package git_block

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestStorage_Submodule runs a simple test of submodule references.
func TestStorage_Submodule(t *testing.T) {
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

	store, err := NewStore(ctx, btx, bcs, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer store.Close()

	// Create a submodule storer.
	subm, err := store.Module("my/submodule")
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = subm

	_, bcs, err = btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("wrote submodule and updated root ref to %s", bcs.GetRef().MarshalString())
}

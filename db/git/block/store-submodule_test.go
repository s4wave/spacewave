package git_block

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestStorage_Submodule runs a simple test of submodule references.
func TestStorage_Submodule(t *testing.T) {
	// Prepare the context and logger for submodule storage.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the submodule testbed with its in-memory volume.
	testbed.Verbose = true
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Report the volume used by the submodule testbed.
	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// Open an empty repository cursor in the testbed.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Initialize the repository block at its transaction cursor.
	btx, bcs := oc.BuildTransaction(nil)
	root := NewRepo()
	bcs.SetBlock(root, true)

	// Open the parent Store for submodule creation.
	store, err := NewStore(ctx, btx, bcs, nil, nil)
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

	// Persist the submodule repository and report its root reference.
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("wrote submodule and updated root ref to %s", bcs.GetRef().MarshalString())
}

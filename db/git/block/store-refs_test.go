package git_block

import (
	"context"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestStorage_References runs a simple test of storing references.
func TestStorage_References(t *testing.T) {
	// Prepare the context and logger for Git reference storage.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the reference testbed with its in-memory volume.
	testbed.Verbose = true
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Report the volume used by the reference testbed.
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

	// Open the Store for Git reference writes and reads.
	store, err := NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer store.Close()

	// Confirm missing references return ErrReferenceNotFound.
	_, err = store.Reference("notfound")
	if err != plumbing.ErrReferenceNotFound {
		t.Fail()
	}

	// Store a symbolic reference in the repository.
	testRef := plumbing.ReferenceName("main")
	err = store.SetReference(plumbing.NewSymbolicReference(testRef, "master"))
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Info("set reference 'main'")

	// Read back the reference before persisting the transaction.
	_, err = store.Reference(testRef)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Persist the reference root and reopen the store.
	rootRef, bcs, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("wrote new root node %s", rootRef.MarshalString())

	// Reopen the Store at its persisted reference root.
	store, err = NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer store.Close()

	// Verify the reference remains after reopening the store.
	ref, err := store.Reference(testRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ref.Name() != testRef {
		t.Fail()
	}
}

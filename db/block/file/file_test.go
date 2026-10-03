package file

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestFile_Basic runs a basic file end to end test.
func TestFile_Basic(t *testing.T) {
	// Prepare the context and logger for the file testbed.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start a verbose storage testbed for file readback.
	testbed.Verbose = true
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Report the testbed volume used for the file fixture.
	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// Open an empty object cursor in the testbed volume.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a file transaction, handle, and writer.
	btx, bcs := oc.BuildTransaction(nil)
	root := &File{}
	bcs.SetBlock(root, true)
	handle := NewHandle(ctx, bcs, root)
	wr := NewWriter(handle, btx, nil)

	// Write the test contents and verify the complete byte count.
	dat := []byte("hello world")
	n, err := wr.Write(dat)
	if err == nil && n != len(dat) {
		err = errors.Errorf("expected write %d but wrote %d", len(dat), n)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Publish the file and verify it has a block reference.
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(bcs.GetRef().GetHash().GetHash()) == 0 {
		t.Fail()
	}
	t.Logf("wrote %q to ref %q", string(dat), bcs.GetRef().MarshalString())

	// Reopen the object cursor at the published file root.
	oc.SetRootRef(bcs.GetRef())
	_, bcs = oc.BuildTransaction(nil)

	// Decode the published file root.
	fi, err := block.UnmarshalBlock[*File](ctx, bcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the reopened file and verify its contents.
	handle = NewHandle(ctx, bcs, fi)
	readDat, err := io.ReadAll(handle)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(readDat, dat) {
		t.Fatalf("data inconsistency: expected %q got %q", string(dat), string(readDat))
	}
	t.Log("successfully read identical data from file")
}

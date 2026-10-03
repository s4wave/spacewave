package unixfs_block

import (
	"bytes"
	"context"
	"embed"
	"strings"
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/file"
	"github.com/s4wave/spacewave/db/testbed"
)

//go:embed fs
var testFS embed.FS

func TestCreate(t *testing.T) {
	ctx := context.Background()

	writeTs := timestamp.Now()
	success := testbed.RunSubtest(t, "CopyFSToFSTree", func(t *testing.T, tb *testbed.Testbed) {
		// Open an empty object cursor for the embedded filesystem copy.
		bls, err := tb.BuildEmptyCursor(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Initialize the destination filesystem root in a block transaction.
		btx, bcs := bls.BuildTransaction(nil)
		bcs.SetBlock(NewFSNode(NodeType_NodeType_DIRECTORY, 0, writeTs.CloneVT()), true)
		fsTree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Copy the embedded filesystem into the destination tree.
		err = CopyFSToFSTree(ctx, testFS, fsTree, nil, writeTs.CloneVT())
		if err != nil {
			t.Fatal(err.Error())
		}

		// Persist the copied filesystem and record its root reference.
		var ref *block.BlockRef
		ref, bcs, err = btx.Write(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		tb.Logger.Infof("wrote test filesystem to block: %s", ref.MarshalString())

		// Reload the saved filesystem and locate its copied source file.
		fsTree, err = NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
		if err != nil {
			t.Fatal(err.Error())
		}
		childTree, _, err := fsTree.LookupFollowDirent("fs")
		if err != nil {
			t.Fatal(err.Error())
		}
		fileEnt, _, err := childTree.LookupFollowDirent("fs.go")
		if err != nil {
			t.Fatal(err.Error())
		}

		// Open the copied file and fetch its stored bytes.
		fh, err := fileEnt.BuildFileHandle(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		var buf bytes.Buffer
		err = file.FetchToBuffer(ctx, fh.GetCursor(), &buf)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Verify the copied source retains its size and expected contents.
		if buf.Len() < 1000 {
			t.Fatalf("expected fs.go to be at least 1000 bytes but got %d", buf.Len())
		}
		bufStr := buf.String()
		if !strings.Contains(bufStr, "UpdateRootRef") {
			t.Fatalf("expected fs.go to contain UpdateRootRef but didn't: %v", bufStr)
		}
	})
	if !success {
		t.Fail()
	}
}

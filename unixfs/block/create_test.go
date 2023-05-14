package unixfs_block

import (
	"bytes"
	"context"
	"embed"
	"strings"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/file"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/timestamp"
)

//go:embed fs
var testFS embed.FS

func TestCreate(t *testing.T) {
	ctx := context.Background()

	writeTs := timestamp.Now()
	success := testbed.RunSubtest(t, "CopyFSToFSTree", func(t *testing.T, tb *testbed.Testbed) {
		bls, err := tb.BuildEmptyCursor(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		btx, bcs := bls.BuildTransaction(nil)
		bcs.SetBlock(NewFSNode(NodeType_NodeType_DIRECTORY, 0, &writeTs), true)
		fsTree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
		if err != nil {
			t.Fatal(err.Error())
		}
		err = CopyFSToFSTree(ctx, testFS, fsTree, nil, &writeTs)
		if err != nil {
			t.Fatal(err.Error())
		}
		var ref *block.BlockRef
		ref, bcs, err = btx.Write(true)
		if err != nil {
			t.Fatal(err.Error())
		}
		tb.Logger.Infof("wrote test filesystem to block: %s", ref.MarshalString())
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
		fh, err := fileEnt.BuildFileHandle(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		var buf bytes.Buffer
		err = file.FetchToBuffer(ctx, fh.GetCursor(), &buf)
		if err != nil {
			t.Fatal(err.Error())
		}
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

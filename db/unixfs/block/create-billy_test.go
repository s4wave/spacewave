//go:build !js

package unixfs_block

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/file"
	"github.com/s4wave/spacewave/db/testbed"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6/osfs"
)

func TestCreateBilly(t *testing.T) {
	ctx := context.Background()

	writeTs := timestamp.Now()
	success := testbed.RunSubtest(t, "CopyBillyFSToFSTree", func(t *testing.T, tb *testbed.Testbed) {
		bls, err := tb.BuildEmptyCursor(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		btx, bcs := bls.BuildTransaction(nil)
		bcs.SetBlock(NewFSNode(NodeType_NodeType_DIRECTORY, 0, writeTs.CloneVT()), true)
		fsTree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
		if err != nil {
			t.Fatal(err.Error())
		}
		bfs := osfs.New("./fs", osfs.WithChrootOS())
		err = CopyBillyFSToFSTree(ctx, bfs, fsTree, nil, writeTs.CloneVT())
		if err != nil {
			t.Fatal(err.Error())
		}
		var ref *block.BlockRef
		ref, bcs, err = btx.Write(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		tb.Logger.Infof("wrote test filesystem to block: %s", ref.MarshalString())
		fsTree, err = NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
		if err != nil {
			t.Fatal(err.Error())
		}
		fileEnt, _, err := fsTree.LookupFollowDirent("fs.go")
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

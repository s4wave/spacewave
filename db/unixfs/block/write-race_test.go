package unixfs_block

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block/file"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
)

// TestWriteAtRootConcurrentDirentNoRace exercises the WriteAtRoot
// concurrent encode pool over a wide unixfs directory whose Dirent
// entries are repeatedly rewritten, so many sibling sub-blocks apply
// their computed BlockRef into shared parent block memory in one write.
// It guards against a SizeVT / MarshalToSizedBufferVT data race on a
// shared *Dirent during concurrent encode (issue: unixfs Dirent
// concurrent marshal panic). Run with -race. It exercises the encode
// pool rather than isolating the lazy non-dirty sub-block trigger, so it
// is a regression exercise, not a standalone falsifier for that path.
func TestWriteAtRootConcurrentDirentNoRace(t *testing.T) {
	ctx := context.Background()
	const fileCount = 48

	testbed.RunSubtest(t, "ConcurrentDirentRewrite", func(t *testing.T, tb *testbed.Testbed) {
		bls, err := tb.BuildEmptyCursor(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		btx, bcs := bls.BuildTransaction(nil)
		bcs.SetBlock(NewFSNode(NodeType_NodeType_DIRECTORY, 0, nil), true)
		fsTree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
		if err != nil {
			t.Fatal(err.Error())
		}

		seedAll := func(tree *FSTree) {
			w := NewFSWriter(tree)
			for i := range fileCount {
				name := "f" + strconv.Itoa(i)
				err := w.MknodWithContent(
					ctx,
					[]string{name},
					unixfs.NewFSCursorNodeType_File(),
					int64(len("seed")),
					bytes.NewReader([]byte("seed")),
					0o644,
					time.Unix(1, 0),
				)
				if err != nil {
					t.Fatal(err.Error())
				}
			}
		}

		rewriteAll := func(tree *FSTree, payload []byte) {
			for i := range fileCount {
				name := "f" + strconv.Itoa(i)
				ent, _, err := tree.LookupFollowDirent(name)
				if err != nil {
					t.Fatal(err.Error())
				}
				fh, err := ent.BuildFileHandle(ctx)
				if err != nil {
					t.Fatal(err.Error())
				}
				if err := file.NewWriter(fh, btx, nil).WriteBytes(0, payload); err != nil {
					t.Fatal(err.Error())
				}
			}
		}

		seedAll(fsTree)
		if _, bcs, err = btx.Write(ctx, true); err != nil {
			t.Fatal(err.Error())
		}

		for iter := range 40 {
			fsTree, err = NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
			if err != nil {
				t.Fatal(err.Error())
			}
			rewriteAll(fsTree, bytes.Repeat([]byte{byte('a' + iter%26)}, iter+1))
			if _, bcs, err = btx.Write(ctx, true); err != nil {
				t.Fatal(err.Error())
			}
		}
	})
}

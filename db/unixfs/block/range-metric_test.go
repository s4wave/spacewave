package unixfs_block

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/block/file"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

func TestUnixFSBlockRangeMetricWriteAtWriteBlobTruncate(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewFSNode(NodeType_NodeType_DIRECTORY, 0, nil), true)
	root, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := Mknod(root, [][]string{{"metric-writeat"}}, NodeType_NodeType_FILE, 0, nil); err != nil {
		t.Fatal(err.Error())
	}
	if err := Mknod(root, [][]string{{"metric-writeblob"}}, NodeType_NodeType_FILE, 0, nil); err != nil {
		t.Fatal(err.Error())
	}
	first := bytes.Repeat([]byte("unixfs-writeat-"), 96)
	second := bytes.Repeat([]byte("blob-append-"), 48)
	truncateTail := make([]byte, 16)
	expected := append(append(append([]byte(nil), first...), second...), truncateTail...)

	if err := WriteAt(ctx, root, nil, []string{"metric-writeat"}, 0, int64(len(first)), bytes.NewReader(first), nil); err != nil {
		t.Fatal(err.Error())
	}
	if err := WriteAt(ctx, root, nil, []string{"metric-writeat"}, int64(len(first)), int64(len(second)), bytes.NewReader(second), nil); err != nil {
		t.Fatal(err.Error())
	}
	if err := TruncateFile(ctx, root, []string{"metric-writeat"}, int64(len(expected)), nil); err != nil {
		t.Fatal(err.Error())
	}

	if err := WriteAt(ctx, root, nil, []string{"metric-writeblob"}, 0, int64(len(first)), bytes.NewReader(first), nil); err != nil {
		t.Fatal(err.Error())
	}
	blobBtx, blobBcs := oc.BuildTransaction(nil)
	_, err = blob.BuildBlob(ctx, int64(len(second)), bytes.NewReader(second), blobBcs, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	blobRef, _, err := blobBtx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := WriteBlob(ctx, root, []string{"metric-writeblob"}, int64(len(first)), blobRef, true, false, nil); err != nil {
		t.Fatal(err.Error())
	}
	if err := TruncateFile(ctx, root, []string{"metric-writeblob"}, int64(len(expected)), nil); err != nil {
		t.Fatal(err.Error())
	}

	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	root, err = NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	readMetricFile := func(name string) ([]byte, int) {
		child, _, err := root.LookupFollowDirent(name)
		if err != nil {
			t.Fatal(err.Error())
		}
		fh, err := child.BuildFileHandle(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer fh.Close()
		out, err := io.ReadAll(fh)
		if err != nil {
			t.Fatal(err.Error())
		}
		rootFile, err := block.UnmarshalBlock[*file.File](ctx, fh.GetCursor(), file.NewFileBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		return out, len(rootFile.GetRanges())
	}
	writeAtOut, writeAtRanges := readMetricFile("metric-writeat")
	if !bytes.Equal(writeAtOut, expected) {
		t.Fatal("unixfs WriteAt append metric readback mismatch")
	}
	writeBlobOut, writeBlobRanges := readMetricFile("metric-writeblob")
	if !bytes.Equal(writeBlobOut, expected) {
		t.Fatal("unixfs WriteBlob append metric readback mismatch")
	}
	if writeAtRanges != writeBlobRanges {
		t.Fatalf("unixfs append parity range mismatch: writeat=%d writeblob=%d", writeAtRanges, writeBlobRanges)
	}
	t.Logf("metric workload=unixfs-write-boundary write_at_bytes=%d write_at_append_bytes=%d write_blob_bytes=%d truncate_size=%d write_at_range_count=%d write_blob_range_count=%d", len(first), len(second), len(second), len(expected), writeAtRanges, writeBlobRanges)
}

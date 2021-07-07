package unixfs_block

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/file"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestBasicDirectory is a simple directory test.
func TestBasicDirectory(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

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

	ftree, err := NewFSTree(bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}
	dirents, err := ReaddirAll(ctx, ftree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(dirents) != 0 {
		t.Fail()
	}

	// test adding directory entries
	t.Log(ftree.GetCursorRef().MarshalString())
	cursors, err := ftree.Mkdir("test-directory")
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(cursors) != 1 {
		t.Fail()
	}
	cursors2, err := ftree.Mkdir("test-directory")
	if err != nil {
		t.Fatal(err.Error())
	}
	b1, _ := cursors["test-directory"].bcs.GetBlock()
	b2, _ := cursors2["test-directory"].bcs.GetBlock()
	if b1 != b2 {
		t.Fail()
	}

	_, bcs, err = btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	ftree, err = NewFSTree(bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	dirents, err = ReaddirAll(ctx, ftree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(dirents) != 1 ||
		dirents["test-directory"].GetNodeType() != NodeType_NodeType_DIRECTORY {
		t.Fail()
	}
}

func TestEmptyFstree(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

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

	btx, bcs := oc.BuildTransactionAtRef(nil, &block.BlockRef{})
	ftree, err := NewFSTree(bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}
	dirents, err := ReaddirAll(ctx, ftree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(dirents) != 0 {
		t.Fail()
	}

	de, err := ftree.Lookup("noexist")
	if err != nil {
		t.Fatal(err.Error())
	}
	if de != nil {
		t.Fail()
	}

	// test adding directory entries
	t.Log(ftree.GetCursorRef().MarshalString())
	cursors, err := ftree.Mkdir("test-directory")
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(cursors) != 1 {
		t.Fail()
	}
	cursors2, err := ftree.Mkdir("test-directory")
	if err != nil {
		t.Fatal(err.Error())
	}
	b1, _ := cursors["test-directory"].bcs.GetBlock()
	b2, _ := cursors2["test-directory"].bcs.GetBlock()
	if b1 != b2 {
		t.Fail()
	}

	_, bcs, err = btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	ftree, err = NewFSTree(bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	dirents, err = ReaddirAll(ctx, ftree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(dirents) != 1 ||
		dirents["test-directory"].GetNodeType() != NodeType_NodeType_DIRECTORY {
		t.Fail()
	}
}

// TestBasicFile is a simple file test.
func TestBasicFile(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

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
	ftree, err := NewFSTree(bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, err = ftree.Mkdir("test-directory")
	if err != nil {
		t.Fatal(err.Error())
	}
	_, bcs, err = btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	ftree, err = NewFSTree(bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	childFtree, err := ftree.Mknod("test-file", NodeType_NodeType_FILE, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	fh, err := childFtree.BuildFileHandle(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	fhw := file.NewWriter(fh, btx, nil)
	err = fhw.WriteBytes(0, []byte("test 1234"))
	if err != nil {
		t.Fatal(err.Error())
	}
	_, bcs, err = btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	/*
		ftree, err = NewFSTree(bcs, NodeType_NodeType_DIRECTORY)
		if err != nil {
			t.Fatal(err.Error())
		}
	*/
}

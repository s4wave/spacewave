package file

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TestFile_Basic runs a basic file end to end test.
func TestFile_Basic(t *testing.T) {
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
	root := &File{}
	bcs.SetBlock(root, true)
	handle := NewHandle(ctx, bcs, root)
	wr := NewWriter(handle, btx, nil)

	dat := []byte("hello world")
	n, err := wr.Write(dat)
	if err == nil && n != len(dat) {
		err = errors.Errorf("expected write %d but wrote %d", len(dat), n)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	_, bcs, err = btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(bcs.GetRef().GetHash().GetHash()) == 0 {
		t.Fail()
	}
	t.Logf("wrote %q to ref %q", string(dat), bcs.GetRef().MarshalString())

	oc.SetRootRef(bcs.GetRef())
	_, bcs = oc.BuildTransaction(nil)
	fi, err := bcs.Unmarshal(NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	handle = NewHandle(ctx, bcs, fi.(*File))

	readDat, err := ioutil.ReadAll(handle)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(readDat, dat) {
		t.Fatalf("data inconsistency: expected %q got %q", string(dat), string(readDat))
	}
	t.Log("successfully read identical data from file")
}

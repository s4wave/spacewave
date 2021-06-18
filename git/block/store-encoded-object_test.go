package git_block

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/sirupsen/logrus"
)

// TestStorage_EncodedObject runs a simple test of storing encoded objects.
func TestStorage_EncodedObject(t *testing.T) {
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
	root := NewRepo()
	bcs.SetBlock(root, true)

	store, err := NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer store.Close()

	// check not exists
	var ohash plumbing.Hash
	copy(ohash[:], []byte("notfound"))
	_, err = store.EncodedObject(plumbing.BlobObject, ohash)
	if err != plumbing.ErrObjectNotFound {
		t.Fail()
	}

	objData := []byte("hello world")
	putObject := func(store *Store, objData []byte) plumbing.Hash {
		encObj := store.NewEncodedObject()
		encObj.SetType(plumbing.BlobObject)
		wc, err := encObj.Writer()
		if err != nil {
			t.Fatal(err.Error())
		}
		n, err := wc.Write(objData)
		if err != nil {
			t.Fatal(err.Error())
		}
		if n != len(objData) {
			t.Fail()
		}

		ph, err := store.SetEncodedObject(encObj)
		if err != nil {
			t.Fatal(err.Error())
		}
		le.Infof("wrote encoded object with hash %s", ph.String())
		return ph
	}

	getObject := func(store *Store, ph plumbing.Hash) plumbing.EncodedObject {
		encObj, err := store.EncodedObject(plumbing.BlobObject, ph)
		if err != nil {
			t.Fatal(err.Error())
		}
		rc, err := encObj.Reader()
		if err != nil {
			t.Fatal(err.Error())
		}
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err.Error())
		}
		_ = rc.Close()
		if len(objData) != 0 && bytes.Compare(data, objData) != 0 {
			t.Fail()
		}
		le.Infof("read & validated encoded object %s", encObj.Hash().String())
		return encObj
	}

	// put an object
	ph := putObject(store, objData)

	// read the object back
	encObj := getObject(store, ph)

	// commit the tx
	var storeRef *block.BlockRef
	storeRef, bcs, err = btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// re-build the store
	oc.SetRootRef(storeRef)
	btx, bcs = oc.BuildTransaction(nil)
	store, err = NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// read the object back
	encObj = getObject(store, ph)

	// check it exists
	size, err := store.EncodedObjectSize(ph)
	if err != nil {
		t.Fatal(err.Error())
	}
	if int(size) != len(objData) {
		t.Fail()
	}

	// success
	_ = encObj
}

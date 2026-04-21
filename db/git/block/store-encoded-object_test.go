package git_block

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/go-git/go-git/v6/plumbing"
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
	ohash, ok := plumbing.FromBytes([]byte("notfound000000000000"))
	if !ok {
		t.Fatal("FromBytes failed")
	}
	_, err = store.EncodedObject(plumbing.BlobObject, ohash)
	if err != plumbing.ErrObjectNotFound {
		t.Fatalf("expected ErrObjectNotFound, got: %v", err)
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
			t.Fatalf("wrote %d bytes, expected %d", n, len(objData))
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
		if len(objData) != 0 && !bytes.Equal(data, objData) {
			t.Fatalf("data mismatch: got %q, want %q", data, objData)
		}
		le.Infof("read & validated encoded object %s", encObj.Hash().String())
		return encObj
	}

	// put an object
	ph := putObject(store, objData)

	// read the object back
	encObj := getObject(store, ph)
	_ = encObj

	// commit via Store (builds IAVL trees from bulk-written objects)
	err = store.Commit()
	if err != nil {
		t.Fatal(err.Error())
	}

	// re-build the store from the committed ref
	storeRef := store.GetRef()
	oc.SetRootRef(storeRef)
	btx, bcs = oc.BuildTransaction(nil)
	store, err = NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer store.Close()

	// read the object back
	encObj = getObject(store, ph)

	// check it exists
	size, err := store.EncodedObjectSize(ph)
	if err != nil {
		t.Fatal(err.Error())
	}
	if int(size) != len(objData) {
		t.Fatalf("size mismatch: got %d, want %d", size, len(objData))
	}

	// success
	_ = encObj
}

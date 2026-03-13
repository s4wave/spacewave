package git_block

import (
	"bytes"
	"io"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/util/iocloser"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/pkg/errors"
)

// StoreEncodedObject is an encoded object attached to a store.
type StoreEncodedObject struct {
	r *Store
	// bcs is the block cursor, nil if not attached to tree.
	bcs *block.Cursor
	// fetched indicates if the blob has been fetched to buf or not.
	fetched bool
	// buf is the buffer containing the fetched data
	buf bytes.Buffer
	// objType is the object type, pending write
	objType plumbing.ObjectType
	// size is the size, pending write
	// note: will be 0 unless SetSize is called
	size int64
}

// NewStoreEncodedObject constructs a new StoreEncodedObject.
// if bcs is nil, indicates the ref has not been committed.
func NewStoreEncodedObject(r *Store, bcs *block.Cursor) *StoreEncodedObject {
	return &StoreEncodedObject{r: r, bcs: bcs}
}

// NewEncodedObject returns a new plumbing.EncodedObject, the real type
// of the object can be a custom implementation or the default one,
// plumbing.MemoryObject.
func (r *Store) NewEncodedObject() plumbing.EncodedObject {
	return NewStoreEncodedObject(r, nil)
}

// rawObjectCloser wraps a writer to store the object on close.
type rawObjectCloser struct {
	store  *Store
	obj    plumbing.EncodedObject
	closer io.Closer
}

// Close closes the writer and stores the object.
func (c *rawObjectCloser) Close() error {
	if err := c.closer.Close(); err != nil {
		return err
	}
	_, err := c.store.SetEncodedObject(c.obj)
	return err
}

// RawObjectWriter returns a writer for writing a raw object.
func (r *Store) RawObjectWriter(typ plumbing.ObjectType, sz int64) (io.WriteCloser, error) {
	obj := r.NewEncodedObject()
	obj.SetType(typ)
	obj.SetSize(sz)
	w, err := obj.Writer()
	if err != nil {
		return nil, err
	}
	return &struct {
		io.Writer
		io.Closer
	}{
		Writer: w,
		Closer: &rawObjectCloser{store: r, obj: obj, closer: w},
	}, nil
}

// SetEncodedObject saves an object into the storage.
func (r *Store) SetEncodedObject(eoi plumbing.EncodedObject) (plumbing.Hash, error) {
	var h plumbing.Hash
	origHash := (*plumbing.Hash)(nil)
	eo, ok := eoi.(*StoreEncodedObject)
	if !ok || eo.r != r || eo.bcs != nil || !eo.fetched {
		eoh := eoi.Hash()
		origHash = &eoh
		eo = r.NewEncodedObject().(*StoreEncodedObject)
		eo.SetType(eoi.Type())
		rc, err := eoi.Reader()
		if err != nil {
			return h, err
		}
		eo.fetched = true
		wc, err := eo.Writer()
		if err != nil {
			return h, err
		}
		if _, err := io.Copy(wc, rc); err != nil {
			return h, err
		}
	}

	h = eo.Hash()
	if origHash != nil {
		if !bytes.Equal(h.Bytes(), origHash.Bytes()) {
			return h, errors.Errorf(
				"hash mismatch when converting: %v != expected %v",
				(*origHash).String(),
				h.String(),
			)
		}
	}

	writeBuf := &eo.buf
	writeLen := int64(writeBuf.Len())
	if eo.size != 0 && eo.size != writeLen {
		return h, errors.Wrapf(
			ErrSizeInvalid,
			"expected %d got %d",
			eo.size, writeLen,
		)
	}

	buildBlobOpts := &blob.BuildBlobOpts{}
	if r.root.EncodedObjectStore == nil {
		r.root.EncodedObjectStore = &EncodedObjectStore{}
	}
	var err error
	buildBlobOpts.ChunkerArgs, err = r.root.EncodedObjectStore.getOrGenerateChunkerArgs()
	if err != nil {
		return h, err
	}

	key, err := r.buildEncodedObjectKey(eo.Type(), h)
	if err != nil {
		return h, err
	}

	ctx := r.ctx

	// Bulk mode: write each object via a per-object mini-transaction,
	// accumulate refs for deferred IAVL tree construction at Commit.
	if r.storeOps != nil {
		tx, encObjCs := block.NewTransaction(r.storeOps, r.bulkXfrm, nil, r.bulkPutOpts)
		encObjCs.ClearAllRefs()
		encObjBlk := &EncodedObject{}
		encObjBlk.DataHash, err = NewHash(h)
		if err != nil {
			return h, err
		}
		encObjBlk.EncodedObjectType = NewEncodedObjectType(eo.Type())
		if err = encObjBlk.Validate(); err != nil {
			return h, err
		}
		encObjCs.SetBlock(encObjBlk, true)

		dataBlobCs := encObjCs.FollowRef(1, nil)
		encObjBlk.DataBlob, err = blob.BuildBlob(ctx, writeLen, writeBuf, dataBlobCs, buildBlobOpts)
		if err != nil {
			return h, err
		}
		if ci := encObjBlk.DataBlob.ChunkIndex; ci != nil {
			encObjBlk.DataBlob.ChunkIndex.ChunkerArgs = nil
		}

		ref, _, err := tx.Write(ctx, true)
		if err != nil {
			return h, err
		}

		r.objIndex[h] = ref
		r.objKeys = append(r.objKeys, bulkEntry{key: key, ref: ref})

		eo.bcs = nil
		eo.fetched = false
		eo.buf.Reset()
		return h, nil
	}

	// Non-bulk mode: write into the IAVL tree directly via btx.
	encTree := r.objTree
	rootCursor := r.objTree.GetCursor()

	encObjCs := rootCursor.Detach(false)
	encObjCs.ClearAllRefs()
	encObjBlk := &EncodedObject{}
	encObjBlk.DataHash, err = NewHash(h)
	if err != nil {
		return h, err
	}
	encObjBlk.EncodedObjectType = NewEncodedObjectType(eo.Type())
	err = encObjBlk.Validate()
	if err != nil {
		return h, err
	}
	encObjCs.SetBlock(encObjBlk, true)

	err = encTree.SetCursorAtKey(ctx, key, encObjCs, false)
	if err != nil {
		return h, err
	}

	dataBlobCs := encObjCs.FollowRef(1, nil)
	encObjBlk.DataBlob, err = blob.BuildBlob(
		ctx,
		writeLen,
		writeBuf,
		dataBlobCs,
		buildBlobOpts,
	)
	if err != nil {
		return h, err
	}
	if ci := encObjBlk.DataBlob.ChunkIndex; ci != nil {
		encObjBlk.DataBlob.ChunkIndex.ChunkerArgs = nil
	}

	eo.bcs = encObjCs
	eo.fetched = false
	eo.buf.Reset()

	return h, nil
}

// EncodedObject gets an object by hash with the given plumbing.ObjectType.
// Implementors should return (nil, plumbing.ErrObjectNotFound) if an object
// doesn't exist with both the given hash and object type.
//
// Valid plumbing.ObjectType values are CommitObject, BlobObject, TagObject,
// TreeObject and AnyObject. If plumbing.AnyObject is given, the object must be
// looked up regardless of its type.
func (r *Store) EncodedObject(ot plumbing.ObjectType, oh plumbing.Hash) (plumbing.EncodedObject, error) {
	if ot == plumbing.AnyObject || ot == 0 {
		return r.EncodedObjectByHash(oh)
	}

	// Check bulk index for objects written but not yet committed to IAVL.
	if r.objIndex != nil {
		if cs := r.lookupBulkObject(oh); cs != nil {
			encObj, err := block.UnmarshalBlock[*EncodedObject](r.ctx, cs, NewEncodedObjectBlock)
			if err != nil {
				return nil, err
			}
			if EncodedObjectType(ot) != encObj.GetEncodedObjectType() {
				return nil, plumbing.ErrObjectNotFound
			}
			return NewStoreEncodedObject(r, cs), nil
		}
	}

	key, err := r.buildEncodedObjectKey(ot, oh)
	if err != nil {
		return nil, err
	}
	encObj, encObjCs, err := r.lookupEncodedObject(key)
	if err != nil || encObj == nil {
		return nil, err
	}
	ph, err := FromHash(encObj.GetDataHash())
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(ph.Bytes(), oh.Bytes()) {
		return nil, errors.Wrapf(
			ErrHashMismatch,
			"expected %v got %v",
			oh.String(),
			ph.String(),
		)
	}
	encObjType := encObj.GetEncodedObjectType()
	if eot := EncodedObjectType(ot); eot != encObjType {
		return nil, errors.Errorf(
			"storage: expected object type %s but got %s",
			eot.String(),
			encObjType.String(),
		)
	}
	return NewStoreEncodedObject(r, encObjCs), nil
}

// EncodedObjectByHash looks up an encoded object by hash only.
// Returns plumbing.ErrObjectNotFound if not found.
func (r *Store) EncodedObjectByHash(ph plumbing.Hash) (plumbing.EncodedObject, error) {
	// Check bulk index for objects written but not yet committed to IAVL.
	if r.objIndex != nil {
		if cs := r.lookupBulkObject(ph); cs != nil {
			return NewStoreEncodedObject(r, cs), nil
		}
	}

	for i := EncodedObjectType(1); i <= EncodedObjectType_EncodedObjectType_MAX; i++ {
		encObj, err := r.EncodedObject(i.ToObjectType(), ph)
		if err != nil {
			if err != plumbing.ErrObjectNotFound {
				return nil, err
			}
		} else if encObj != nil {
			return encObj, nil
		}
	}
	return nil, plumbing.ErrObjectNotFound
}

// IterObjects returns a custom EncodedObjectStorer over all the object
// on the storage.
//
// Valid plumbing.ObjectType values are CommitObject, BlobObject, TagObject,
func (r *Store) IterEncodedObjects(ph plumbing.ObjectType) (storer.EncodedObjectIter, error) {
	var prefix []byte
	if ph != plumbing.AnyObject && ph != plumbing.InvalidObject {
		prefix = []byte{byte(ph)} //nolint:gosec
	}
	treeTx := r.objTree
	ktxIterator := treeTx.BlockIterate(r.ctx, prefix, false, false)
	return NewEncodedObjectIter(r, ktxIterator), nil
}

// HasEncodedObject returns ErrObjNotFound if the object doesn't
// exist.  If the object does exist, it returns nil.
func (r *Store) HasEncodedObject(ph plumbing.Hash) error {
	if r.objIndex != nil {
		if _, ok := r.objIndex[ph]; ok {
			return nil
		}
	}
	_, err := r.EncodedObjectByHash(ph)
	return err
}

// EncodedObjectSize returns the plaintext size of the encoded object.
func (r *Store) EncodedObjectSize(ph plumbing.Hash) (int64, error) {
	encObj, err := r.EncodedObjectByHash(ph)
	if err != nil {
		return 0, err
	}
	return encObj.Size(), nil
}

// AddAlternate adds an alternate remote.
func (r *Store) AddAlternate(remote string) error {
	// TODO https://stackoverflow.com/questions/36123655/what-is-the-git-alternates-mechanism
	return git.ErrAlternatePathNotSupported
}

// Hash returns the hash of the encoded object.
// Returns empty hash if any errors.
func (o *StoreEncodedObject) Hash() plumbing.Hash {
	h, _ := o.StoreHash()
	oh, _ := FromHash(h)
	return oh
}

// StoreHash returns the hash of the encoded object in storage format.
func (o *StoreEncodedObject) StoreHash() (*hash.Hash, error) {
	if o.fetched || o.bcs == nil {
		// hash the data
		data := (&o.buf).Bytes()
		oh, _ := gitObjectHasher.Compute(o.objType, data)
		return NewHash(oh)
	}
	encObj, err := o.unmarshalEncodedObject()
	if err != nil {
		return nil, err
	}
	return encObj.GetDataHash(), nil
}

// Type returns the Git object type
func (o *StoreEncodedObject) Type() plumbing.ObjectType {
	if o.objType != 0 {
		return o.objType
	}
	if o.bcs == nil {
		return plumbing.InvalidObject
	}
	encObjBlk, err := o.unmarshalEncodedObject()
	if err != nil {
		return plumbing.InvalidObject
	}
	o.objType = encObjBlk.GetEncodedObjectType().ToObjectType()
	return o.objType
}

// SetType sets the git object type.
func (o *StoreEncodedObject) SetType(ot plumbing.ObjectType) {
	o.objType = ot
}

// Size returns the size of the encoded object.
// Note: this is the size of the stored data, not the block graph.
func (o *StoreEncodedObject) Size() int64 {
	if o.fetched || o.bcs == nil {
		return int64((&o.buf).Len())
	}
	encObjBlk, err := o.unmarshalEncodedObject()
	if err != nil {
		return 0
	}
	return int64(encObjBlk.GetDataBlob().GetTotalSize()) //nolint:gosec
}

// SetSize sets the total expected size of the object data.
func (o *StoreEncodedObject) SetSize(s int64) {
	if s < 0 {
		s = 0
	}
	o.size = s
}

// Reader returns the data reader.
func (o *StoreEncodedObject) Reader() (io.ReadCloser, error) {
	if o.fetched {
		return iocloser.NewReadCloser(&o.buf, nil), nil
	}
	if o.bcs == nil {
		// uninitialized encoded object
		return nil, io.EOF
	}
	blk, err := block.UnmarshalBlock[*EncodedObject](o.r.ctx, o.bcs, NewEncodedObjectBlock)
	if err != nil {
		return nil, err
	}
	br, err := blk.BuildDataBlobReader(o.r.ctx, o.bcs)
	if err != nil {
		return nil, err
	}
	return br, nil
}

// Writer returns the data writer.
func (o *StoreEncodedObject) Writer() (io.WriteCloser, error) {
	o.fetched = true
	return iocloser.NewWriteCloser(&o.buf, nil), nil
}

// buildEncodedObjectKey builds the key for an encoded object.
func (r *Store) buildEncodedObjectKey(ot plumbing.ObjectType, h plumbing.Hash) ([]byte, error) {
	if ot == 0 || ot > 7 {
		return nil, errors.Wrapf(ErrObjectTypeInvalid, "%v", ot)
	}
	if h.IsZero() {
		return nil, errors.New("encoded object hash cannot be empty")
	}
	// prefix with hash type
	return append([]byte{byte(ot)}, h.Bytes()...), nil //nolint:gosec
}

// unmarshalEncodedObject unmarshals the EncodedObject block.
// returns nil, nil, if empty
func (o *StoreEncodedObject) unmarshalEncodedObject() (*EncodedObject, error) {
	return block.UnmarshalBlock[*EncodedObject](o.r.ctx, o.bcs, NewEncodedObjectBlock)
}

// lookupEncodedObject tries to build the EncodedObject from a key.
func (r *Store) lookupEncodedObject(key []byte) (*EncodedObject, *block.Cursor, error) {
	encTree := r.objTree
	nodCs, err := encTree.GetCursorAtKey(r.ctx, key)
	if err != nil {
		return nil, nil, err
	}
	if nodCs == nil {
		return nil, nil, plumbing.ErrObjectNotFound
	}
	encObji, err := nodCs.Unmarshal(r.ctx, NewEncodedObjectBlock)
	if err != nil {
		return nil, nil, err
	}
	encObjBlk, ok := encObji.(*EncodedObject)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return encObjBlk, nodCs, nil
}

// _ is a type assertion
var (
	// EncodedObjectStorer stores objects.
	_ storer.EncodedObjectStorer = (*Store)(nil)
	_ plumbing.EncodedObject     = (*StoreEncodedObject)(nil)
)

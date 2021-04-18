package git

import (
	"bytes"
	"io"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/aperturerobotics/hydra/util/closer"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
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
		if bytes.Compare(h[:], (*origHash)[:]) != 0 {
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
	buildBlobOpts.ChunkingPol, err = r.root.EncodedObjectStore.getOrGenerateChunkerPoly()
	if err != nil {
		return h, err
	}

	key, err := r.buildEncodedObjectKey(eo.Type(), h)
	if err != nil {
		return h, err
	}

	// 1. NewEncodedObject, or some other EncodedObject is built
	// 2. eo.Writer() -> returns the bytes.Buffer and sets fetched=true
	// 3. Write() -> stores into in-memory buffer
	// 4. SetEncodedObject:
	//   - transfer data to a EncodedObject w/ Buffer if necessary
	//   - use Set() to create the node for the key in the graph
	//   - use GetWithCursor() to return the cursor at the sub-block
	//   - flush the buffer to a blob with BuildBlob rooted at the sub-block
	//   - (the data is written to the block graph)
	//   - clear the buffer (future Reader() calls will use the block graph)

	ctx := r.ctx
	encTree := r.objTree
	_, nodCs, err := encTree.SetCursorAsRef(key, nil)
	if err != nil {
		return h, err
	}

	encObjCs := nodCs.FollowRef(1, nil)
	encObjBlk := NewEncodedObjectBlock().(*EncodedObject)
	encObjBlk.DataHash, err = NewHash(h)
	encObjBlk.EncodedObjectType = NewEncodedObjectType(eo.Type())
	err = encObjBlk.Validate()
	if err != nil {
		return h, err
	}
	encObjCs.SetBlock(encObjBlk, true)

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
	if encObjBlk.DataBlob.ChunkIndex != nil {
		encObjBlk.DataBlob.ChunkIndex.Pol = 0
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
	if bytes.Compare(ph[:], oh[:]) != 0 {
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
		prefix = []byte{byte(ph)}
	}
	treeTx := r.objTree
	ktxIterator := treeTx.IterateIavl(prefix, false, false)
	return NewEncodedObjectIter(r, ktxIterator), nil
}

// HasEncodedObject returns ErrObjNotFound if the object doesn't
// exist.  If the object does exist, it returns nil.
func (r *Store) HasEncodedObject(ph plumbing.Hash) error {
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
		oh := plumbing.ComputeHash(o.objType, data)
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
	return int64(encObjBlk.GetDataBlob().GetTotalSize())
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
		return closer.NewReadCloser(&o.buf, nil), nil
	}
	if o.bcs == nil {
		// uninitialized encoded object
		return nil, io.EOF
	}
	blki, err := o.bcs.Unmarshal(NewEncodedObjectBlock)
	if err != nil {
		return nil, err
	}
	blk, ok := blki.(*EncodedObject)
	if !ok {
		return nil, block.ErrUnexpectedType
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
	return closer.NewWriteCloser(&o.buf, nil), nil
}

// buildEncodedObjectKey builds the key for an encoded object.
func (r *Store) buildEncodedObjectKey(ot plumbing.ObjectType, h plumbing.Hash) ([]byte, error) {
	if ot == 0 || ot > 7 {
		return nil, errors.Wrapf(ErrObjectTypeInvalid, "%v", ot)
	}
	any := false
	for _, v := range h {
		if v != 0 {
			any = true
			break
		}
	}
	if !any {
		return nil, errors.New("encoded object hash cannot be empty")
	}
	// prefix with hash type
	return append([]byte{byte(ot)}, h[:]...), nil
}

// unmarshalEncodedObject unmarshals the EncodedObject block.
// returns nil, nil, if empty
func (e *StoreEncodedObject) unmarshalEncodedObject() (*EncodedObject, error) {
	if e.bcs == nil {
		return nil, nil
	}
	enci, err := e.bcs.Unmarshal(NewEncodedObjectBlock)
	if err != nil {
		return nil, err
	}
	enc, ok := enci.(*EncodedObject)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return enc, nil
}

// lookupEncodedObject tries to build the EncodedObject from a key.
func (r *Store) lookupEncodedObject(key []byte) (*EncodedObject, *block.Cursor, error) {
	encTree := r.objTree
	_, nodCs, err := encTree.GetWithCursor(key)
	if err != nil {
		return nil, nil, err
	}
	if nodCs == nil {
		return nil, nil, plumbing.ErrObjectNotFound
	}

	nodRef, err := byteslice.ByteSliceToRef(nodCs)
	if err != nil {
		return nil, nil, err
	}
	encObjCs := nodCs.FollowRef(1, nodRef)
	encObji, err := encObjCs.Unmarshal(NewEncodedObjectBlock)
	if err != nil {
		return nil, nil, err
	}
	encObjBlk, ok := encObji.(*EncodedObject)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return encObjBlk, encObjCs, nil
}

// _ is a type assertion
var (
	// EncodedObjectStorer stores objects.
	_ storer.EncodedObjectStorer = (*Store)(nil)
	_ plumbing.EncodedObject     = (*StoreEncodedObject)(nil)
)

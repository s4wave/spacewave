package kvtx_genji

import (
	"bytes"
	"context"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	gengine "github.com/genjidb/genji/engine"
	"github.com/golang/protobuf/proto"
)

// Tx implements the GenjiDB t/x interface with a kvtx tx.
type Tx struct {
	rb    uint32
	ctx   context.Context
	c     context.CancelFunc
	tx    kvtx.Tx
	write bool
}

// NewTx constructs a new Tx.
func NewTx(ctx context.Context, tx kvtx.Tx, write bool) *Tx {
	sctx, sctxCancel := context.WithCancel(ctx)
	return &Tx{
		ctx:   sctx,
		c:     sctxCancel,
		tx:    tx,
		write: write,
	}
}

const (
	separator   byte = 0x1F
	storeKey         = "store"
	storePrefix      = 's'
	seqnumKey        = "seq"
)

func buildStoreKey(name []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(storeKey) + 1 + len(name))
	buf.WriteString(storeKey)
	buf.WriteByte(separator)
	buf.Write(name)

	return buf.Bytes()
}

func buildStorePrefixKey(name []byte) []byte {
	prefix := make([]byte, 0, len(name)+3)
	prefix = append(prefix, storePrefix)
	prefix = append(prefix, separator)
	prefix = append(prefix, name...)

	return prefix
}

// Fetch a store by name. If the store doesn't exist, it returns the ErrStoreNotFound error.
func (t *Tx) GetStore(name []byte) (gengine.Store, error) {
	select {
	case <-t.ctx.Done():
		return nil, t.ctx.Err()
	default:
	}

	key := buildStoreKey(name)
	_, found, err := t.tx.Get(key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, gengine.ErrStoreNotFound
	}

	pkey := buildStorePrefixKey(name)
	return NewStore(t, name, pkey), nil
}

// Create a store with the given name. If the store already exists, it returns ErrStoreAlreadyExists.
func (t *Tx) CreateStore(name []byte) error {
	select {
	case <-t.ctx.Done():
		return t.ctx.Err()
	default:
	}

	if !t.write {
		return gengine.ErrTransactionReadOnly
	}

	key := buildStoreKey(name)
	_, found, err := t.tx.Get(key)
	if err != nil {
		return err
	}
	if found {
		return gengine.ErrStoreAlreadyExists
	}

	meta := NewStoreMeta(time.Now())
	md, err := proto.Marshal(meta)
	if err != nil {
		return err
	}

	return t.tx.Set(key, md, time.Duration(0))
}

// Drop a store by name. If the store doesn't exist, it returns ErrStoreNotFound.
// It deletes all the values stored in it.
func (t *Tx) DropStore(name []byte) error {
	select {
	case <-t.ctx.Done():
		return t.ctx.Err()
	default:
	}

	if !t.write {
		return gengine.ErrTransactionReadOnly
	}

	s, err := t.GetStore(name)
	if err != nil {
		return err
	}

	err = s.Truncate()
	if err != nil {
		return err
	}
	return t.tx.Delete(buildStoreKey([]byte(name)))
}

// Commit applies all changes made in the transaction.
func (t *Tx) Commit() error {
	select {
	case <-t.ctx.Done():
		t.tx.Discard()
		return t.ctx.Err()
	default:
	}
	defer t.c()
	err := t.tx.Commit(t.ctx)
	if err != nil {
		return err
	}
	// after successful commit, rollback -> return nil
	atomic.StoreUint32(&t.rb, 1)
	return nil
}

// Rollback rolls back the transaction.
// Committed transactions will not be affected by calling Rollback.
func (t *Tx) Rollback() error {
	if atomic.LoadUint32(&t.rb) == 1 {
		return nil
	}
	t.tx.Discard()
	select {
	case <-t.ctx.Done():
		return t.ctx.Err()
	default:
		t.c()
	}
	// this is what the genji lib expects
	// after successful rollback -> return nil
	atomic.StoreUint32(&t.rb, 1)
	return nil
}

// _ is a type assertion
var _ gengine.Transaction = ((*Tx)(nil))

package kvtx

import (
	"bytes"
	"context"
	"time"

	"github.com/aperturerobotics/hydra/object"
	"github.com/aperturerobotics/hydra/object/store"
)

// objectStore implements a object store.
type objectStore struct {
	ctx    context.Context
	k      *KVTx
	prefix []byte
}

// OpenObjectStore opens a object store by ID.
func (k *KVTx) OpenObjectStore(ctx context.Context, id string) (object.ObjectStore, error) {
	prefix := k.kvkey.GetObjectStorePrefixByID(id)
	return &objectStore{prefix: prefix, k: k}, nil
}

// DelObjectStore deletes a object store and all contents by ID.
func (k *KVTx) DelObjectStore(ctx context.Context, id string) error {
	prefix := k.kvkey.GetObjectStorePrefixByID(id)
	s := &objectStore{prefix: prefix, k: k}
	return s.purge(ctx)
}

// Get gets an object by key.
func (s *objectStore) GetObject(key string) (val []byte, found bool, err error) {
	k := s.getObjKey(key)
	tx, err := s.k.store.NewTransaction(false)
	if err != nil {
		return nil, false, err
	}
	defer tx.Discard()
	return tx.Get(k)
}

// Set sets an object by key.
func (s *objectStore) SetObject(key string, val []byte) error {
	k := s.getObjKey(key)
	tx, err := s.k.store.NewTransaction(true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Set(k, val, time.Duration(0)); err != nil {
		return err
	}
	return tx.Commit(s.ctx)
}

// ListKeys lists keys with a given prefix.
func (s *objectStore) ListKeys(prefix string) ([]string, error) {
	pf := bytes.Join([][]byte{s.prefix, []byte(prefix)}, nil)
	t, err := s.k.store.NewTransaction(false)
	if err != nil {
		return nil, err
	}
	defer t.Discard()
	var keys []string
	if err := t.ScanPrefix(pf, func(key []byte) error {
		keys = append(keys, string(key))
		return nil
	}); err != nil {
		return nil, err
	}
	return keys, nil
}

// DeleteObject deletes an object by a key.
func (s *objectStore) DeleteObject(key string) error {
	objKey := s.getObjKey(key)
	t, err := s.k.store.NewTransaction(true)
	if err != nil {
		return err
	}
	defer t.Discard()
	if err := t.Delete(objKey); err != nil {
		return err
	}
	return t.Commit(s.ctx)
}

// getObjKey returns the key for an object.
func (s *objectStore) getObjKey(key string) []byte {
	return bytes.Join([][]byte{
		s.prefix,
		[]byte(key),
	}, nil)
}

// purge purges the object store.
func (s *objectStore) purge(ctx context.Context) error {
	t, err := s.k.store.NewTransaction(true)
	if err != nil {
		return err
	}
	defer t.Discard()
	err = t.ScanPrefix(s.prefix, func(key []byte) error {
		return t.Delete(key)
	})
	if err != nil {
		return err
	}
	return t.Commit(ctx)
}

// _ is a type assertion
var _ object.ObjectStore = ((*objectStore)(nil))

// _ is a type assertion
var _ object_store.Store = ((*KVTx)(nil))

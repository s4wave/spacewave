package kvtx_genji

import (
	"errors"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/genjidb/genji/engine"
	gengine "github.com/genjidb/genji/engine"
)

// Store implements the GenjiDB store interface.
type Store struct {
	t         *Tx
	storeName []byte
	prefixKey []byte
	seq       *uint64
}

// NewStore builds a new Store object.
func NewStore(t *Tx, storeName, prefixKey []byte) *Store {
	return &Store{
		t:         t,
		storeName: storeName,
		prefixKey: prefixKey,
	}
}

// build a long key for each key of a store
// in the form: storePrefix + <sep> + key.
func buildKey(prefix, k []byte) []byte {
	key := make([]byte, 0, len(prefix)+2+len(k))
	key = append(key, prefix...)
	key = append(key, separator)
	// key = append(key, 0)
	key = append(key, k...)
	return key
}

// Get returns a value associated with the given key. If no key is not found, it returns ErrKeyNotFound.
func (s *Store) Get(k []byte) (engine.Item, error) {
	select {
	case <-s.t.ctx.Done():
		return nil, s.t.ctx.Err()
	default:
	}

	key := buildKey(s.prefixKey, k)
	data, found, err := s.t.tx.Get(s.t.ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, gengine.ErrKeyNotFound
	}
	return newItem(s, k, &data), nil
}

// Put stores a key value pair. If it already exists, it overrides it.
func (s *Store) Put(k, v []byte) error {
	if err := s.checkWritePre(); err != nil {
		return err
	}

	if len(v) == 0 {
		// genjidb requires this as per their tests
		return errors.New("cannot store empty value")
	}

	if len(k) == 0 {
		return errors.New("cannot store empty key")
	}

	return s.t.tx.Set(s.t.ctx, buildKey(s.prefixKey, k), v)
}

// Delete a key value pair. If the key is not found, returns ErrKeyNotFound.
func (s *Store) Delete(k []byte) error {
	if err := s.checkWritePre(); err != nil {
		return err
	}

	key := buildKey(s.prefixKey, k)
	_, found, err := s.t.tx.Get(s.t.ctx, key)
	if err != nil {
		return err
	}
	if !found {
		return gengine.ErrKeyNotFound
	}

	return s.t.tx.Delete(s.t.ctx, key)
}

// Truncate deletes all the key value pairs from the store.
func (s *Store) Truncate() error {
	if err := s.checkWritePre(); err != nil {
		return err
	}

	return s.t.tx.ScanPrefix(s.t.ctx, s.prefixKey, func(key, value []byte) error {
		return s.t.tx.Delete(s.t.ctx, key)
	})
}

// Iterator creates an iterator with the given options.
// The initial position depends on the implementation.
func (s *Store) Iterator(opts gengine.IteratorOptions) gengine.Iterator {
	return NewIterator(s, opts)
}

// NextSequence returns a monotonically increasing integer.
func (s *Store) NextSequence() (uint64, error) {
	if err := s.checkWritePre(); err != nil {
		return 0, err
	}

	var seqn uint64
	if s.seq != nil {
		seqn = *s.seq
	} else {
		seqb, found, err := s.t.tx.Get(s.t.ctx, []byte(seqnumKey))
		if err != nil {
			return 0, err
		}
		if found {
			seqn, _ = protobuf_go_lite.ConsumeVarint(seqb)
		} else {
			seqn = 1 // start at 1
		}
	}
	ns := seqn + 1
	seqb := protobuf_go_lite.AppendVarint(nil, ns)
	if err := s.t.tx.Set(s.t.ctx, []byte(seqnumKey), seqb); err != nil {
		return 0, err
	}
	s.seq = &ns
	return seqn, nil
}

// checkWritePre checks conditions for a write
func (s *Store) checkWritePre() error {
	select {
	case <-s.t.ctx.Done():
		return s.t.ctx.Err()
	default:
	}
	if !s.t.write {
		return gengine.ErrTransactionReadOnly
	}
	return nil
}

// _ is a type assertion
var _ gengine.Store = ((*Store)(nil))

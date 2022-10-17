package kvtx_kvtest

import (
	"bytes"
	"context"
	"strconv"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/pkg/errors"
)

// TestAll tests all tests for a kvtx store.
func TestAll(ctx context.Context, ktx kvtx.Store) error {
	tx, err := ktx.NewTransaction(false)
	if err != nil {
		return err
	}

	keys := [][]byte{
		[]byte("ab"),
		[]byte("ba"),
		[]byte("ba1"),
		[]byte("ba2"),
		[]byte("bb"),
		[]byte("c"),
	}

	for _, k := range keys {
		ok, err := tx.Exists(k)
		if err != nil {
			return err
		}
		if ok {
			tx.Discard()
			return errors.Errorf("expected not exist: %s", string(k))
		}
	}
	tx.Discard()

	tx, err = ktx.NewTransaction(true)
	if err != nil {
		return err
	}

	for i := range keys {
		v := []byte(strconv.Itoa(i))
		if err := tx.Set(keys[i], v); err != nil {
			tx.Discard()
			return err
		}
		val, ok, err := tx.Get(keys[i])
		if err != nil {
			return err
		}
		if !ok {
			tx.Discard()
			return errors.Errorf("expected key to exist: %s", string(keys[i]))
		}
		if !bytes.Equal(val, v) {
			tx.Discard()
			return errors.Errorf("mismatch of value for key: %s", string(keys[i]))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	tx, err = ktx.NewTransaction(false)
	if err != nil {
		return err
	}

	for i, k := range keys {
		v := []byte(strconv.Itoa(i))
		val, ok, err := tx.Get(k)
		if err != nil {
			return err
		}
		if !ok {
			tx.Discard()
			return errors.Errorf("expected key to exist: %s", string(k))
		}
		if !bytes.Equal(val, v) {
			tx.Discard()
			return errors.Errorf("mismatch of value for key: %s", string(k))
		}
	}

	tx.Discard()

	tx, err = ktx.NewTransaction(true)
	if err != nil {
		return err
	}

	if err := tx.Delete(keys[0]); err != nil {
		tx.Discard()
		return err
	}

	_, ok, err := tx.Get(keys[0])
	if err == nil && ok {
		err = errors.Errorf("expected key to not exist after delete: %s", string(keys[0]))
	}
	if err != nil {
		tx.Discard()
		return err
	}

	tx.Discard()

	tx, err = ktx.NewTransaction(false)
	if err != nil {
		return err
	}

	val, ok, err := tx.Get(keys[0])
	if err == nil && !ok {
		err = errors.Errorf("expected key to exist after delete was discarded: %s", string(keys[0]))
	}
	if err == nil {
		if !bytes.Equal(val, []byte("0")) {
			err = errors.Errorf("value mismatch for key: %s", string(keys[0]))
		}
	}
	if err != nil {
		tx.Discard()
		return err
	}

	tx.Discard()

	tx, err = ktx.NewTransaction(true)
	if err != nil {
		return err
	}
	if err := tx.Set([]byte("test"), []byte{1, 2, 3, 4}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	tx, err = ktx.NewTransaction(false)
	if err != nil {
		return err
	}
	var ks [][]byte
	err = tx.ScanPrefix([]byte("t"), func(key, val []byte) error {
		k := make([]byte, len(key))
		copy(k, key)
		ks = append(ks, k)
		return nil
	})
	if err != nil {
		return err
	}
	if len(ks) != 1 {
		return errors.Errorf("expected slice len 1: %v", ks)
	}
	if string(ks[0]) != "test" {
		return errors.Errorf("expected single entry 'test' %v", ks[0])
	}
	tx.Discard()

	tx, err = ktx.NewTransaction(false)
	if err != nil {
		return err
	}
	dat, found, err := tx.Get([]byte("test"))
	if err != nil {
		return err
	}
	if !found {
		return errors.New("expected to find key test")
	}
	if !bytes.Equal(dat, []byte{1, 2, 3, 4}) {
		return errors.New("incorrect value in data")
	}
	tx.Discard()

	tx, err = ktx.NewTransaction(true)
	if err != nil {
		return err
	}
	if err := tx.Delete([]byte("test")); err != nil {
		return err
	}
	err = tx.Commit(ctx)
	if err != nil {
		return err
	}

	tx, err = ktx.NewTransaction(false)
	if err != nil {
		return err
	}
	dat, found, err = tx.Get([]byte("test"))
	if err != nil {
		return err
	}
	if found || len(dat) != 0 {
		return errors.New("expected not found")
	}
	tx.Discard()

	tx, err = ktx.NewTransaction(true)
	if err != nil {
		return err
	}
	data := []struct{ k, v []byte }{
		{[]byte("test-1"), []byte("testing-1")},
		{[]byte("test-2"), []byte("testing-2")},
		{[]byte("foo-1"), []byte("foo")},
	}
	for _, x := range data {
		_ = tx.Set(x.k, x.v)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	tx, err = ktx.NewTransaction(false)
	if err != nil {
		return err
	}
	_, err = kvtx.MustGet(tx, []byte("foo-1"))
	if err != nil {
		return err
	}
	it := tx.Iterate([]byte("test-"), true, false)
	vals := 0
	if err := it.Seek(nil); err != nil {
		return err
	}
	for ; it.Valid(); it.Next() {
		vals++
	}
	if err := it.Err(); err != nil {
		return err
	}
	if vals != 2 {
		return errors.Errorf("expected 2 values but got %v", vals)
	}
	tx.Discard()

	// check the empty key behavior
	tx, err = ktx.NewTransaction(true)
	if err != nil {
		return err
	}
	expectedEmpty := func(err error) error {
		return errors.Errorf("expected empty key error but got %v", err)
	}
	if _, _, err := tx.Get([]byte{}); err != kvtx.ErrEmptyKey {
		return expectedEmpty(err)
	}
	if err := tx.Set([]byte{}, []byte("testing")); err != kvtx.ErrEmptyKey {
		return expectedEmpty(err)
	}
	if err := tx.Delete([]byte{}); err != kvtx.ErrEmptyKey {
		return expectedEmpty(err)
	}
	tx.Discard()

	return nil
}

package kvtx_kvtest

import (
	"bytes"
	"context"
	"strconv"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/pkg/errors"
)

// withTx executes a function within a transaction context, ensuring proper cleanup
func withTx(ctx context.Context, ktx kvtx.Store, writable bool, fn func(tx kvtx.Tx) error) error {
	tx, err := ktx.NewTransaction(ctx, writable)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return fn(tx)
}

// TestAll tests all tests for a kvtx store.
func TestAll(ctx context.Context, ktx kvtx.Store) error {
	keys := [][]byte{
		[]byte("ab"),
		[]byte("ba"),
		[]byte("ba1"),
		[]byte("ba2"),
		[]byte("bb"),
		[]byte("c"),
	}

	err := withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		for _, k := range keys {
			ok, err := tx.Exists(ctx, k)
			if err != nil {
				return err
			}
			if ok {
				return errors.Errorf("expected not exist: %s", string(k))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = withTx(ctx, ktx, true, func(tx kvtx.Tx) error {
		for i := range keys {
			v := []byte(strconv.Itoa(i))
			if err := tx.Set(ctx, keys[i], v); err != nil {
				return err
			}
			val, ok, err := tx.Get(ctx, keys[i])
			if err != nil {
				return err
			}
			if !ok {
				return errors.Errorf("expected key to exist: %s", string(keys[i]))
			}
			if !bytes.Equal(val, v) {
				return errors.Errorf("mismatch of value for key: %s", string(keys[i]))
			}
		}
		return tx.Commit(ctx)
	})
	if err != nil {
		return err
	}

	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		for i, k := range keys {
			v := []byte(strconv.Itoa(i))
			val, ok, err := tx.Get(ctx, k)
			if err != nil {
				return err
			}
			if !ok {
				return errors.Errorf("expected key to exist: %s", string(k))
			}
			if !bytes.Equal(val, v) {
				return errors.Errorf("mismatch of value for key: %s", string(k))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = withTx(ctx, ktx, true, func(tx kvtx.Tx) error {
		if err := tx.Delete(ctx, keys[0]); err != nil {
			return err
		}

		_, ok, err := tx.Get(ctx, keys[0])
		if err == nil && ok {
			err = errors.Errorf("expected key to not exist after delete: %s", string(keys[0]))
		}
		if err != nil {
			return err
		}
		// note: we do not commit the txn here
		return nil
	})
	if err != nil {
		return err
	}

	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		val, ok, err := tx.Get(ctx, keys[0])
		if err == nil && !ok {
			err = errors.Errorf("expected key to exist after delete was discarded: %s", string(keys[0]))
		}
		if err == nil {
			if !bytes.Equal(val, []byte("0")) {
				err = errors.Errorf("value mismatch for key: %s", string(keys[0]))
			}
		}
		return err
	})
	if err != nil {
		return err
	}

	err = withTx(ctx, ktx, true, func(tx kvtx.Tx) error {
		if err := tx.Set(ctx, []byte("test"), []byte{1, 2, 3, 4}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	if err != nil {
		return err
	}

	var ks [][]byte
	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		return tx.ScanPrefix(ctx, []byte("t"), func(key, val []byte) error {
			ks = append(ks, bytes.Clone(key))
			return nil
		})
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

	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		dat, found, err := tx.Get(ctx, []byte("test"))
		if err != nil {
			return err
		}
		if !found {
			return errors.New("expected to find key test")
		}
		if !bytes.Equal(dat, []byte{1, 2, 3, 4}) {
			return errors.New("incorrect value in data")
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = withTx(ctx, ktx, true, func(tx kvtx.Tx) error {
		if err := tx.Delete(ctx, []byte("test")); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	if err != nil {
		return err
	}

	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		dat, found, err := tx.Get(ctx, []byte("test"))
		if err != nil {
			return err
		}
		if found || len(dat) != 0 {
			return errors.New("expected not found")
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Setup test data
	testData := []struct{ k, v []byte }{
		{[]byte("a/1"), []byte("val1")},
		{[]byte("a/2"), []byte("val2")},
		{[]byte("a/3"), []byte("val3")},
		{[]byte("b/1"), []byte("val4")},
		{[]byte("b/2"), []byte("val5")},
		{[]byte("c/1"), []byte("val6")},
		{[]byte("foo-1"), []byte("foo")},
		{[]byte("test-1"), []byte("testing-1")},
		{[]byte("test-2"), []byte("testing-2")},
	}

	err = withTx(ctx, ktx, true, func(tx kvtx.Tx) error {
		for _, x := range testData {
			if err := tx.Set(ctx, x.k, x.v); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	})
	if err != nil {
		return err
	}

	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		if _, err := kvtx.MustGet(ctx, tx, []byte("foo-1")); err != nil {
			return err
		}

		it := tx.Iterate(ctx, []byte("test-"), true, false)
		defer it.Close()

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
		return nil
	})
	if err != nil {
		return err
	}

	// check empty value behavior
	emptyKey := []byte("empty-value-test")
	err = withTx(ctx, ktx, true, func(tx kvtx.Tx) error {
		if err := tx.Set(ctx, emptyKey, []byte{}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	if err != nil {
		return err
	}

	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		// verify exists
		exists, err := tx.Exists(ctx, emptyKey)
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("expected key with empty value to exist")
		}
		// verify empty value
		val, ok, err := tx.Get(ctx, emptyKey)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("expected to find key with empty value")
		}
		if len(val) != 0 {
			return errors.Errorf("expected empty value but got length %d", len(val))
		}
		return nil
	})
	if err != nil {
		return err
	}

	// cleanup
	err = withTx(ctx, ktx, true, func(tx kvtx.Tx) error {
		if err := tx.Delete(ctx, emptyKey); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	if err != nil {
		return err
	}

	// check the empty key behavior
	err = withTx(ctx, ktx, true, func(tx kvtx.Tx) error {
		expectedEmpty := func(err error) error {
			return errors.Errorf("expected empty key error but got %v", err)
		}
		if _, _, err := tx.Get(ctx, []byte{}); err != kvtx.ErrEmptyKey {
			return expectedEmpty(err)
		}
		if err := tx.Set(ctx, []byte{}, []byte("testing")); err != kvtx.ErrEmptyKey {
			return expectedEmpty(err)
		}
		if err := tx.Delete(ctx, []byte{}); err != kvtx.ErrEmptyKey {
			return expectedEmpty(err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Test iterator seek behavior
	// Test forward seek with prefix
	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		it := tx.Iterate(ctx, []byte("a/"), true, false)
		defer it.Close()
		if err := it.Seek([]byte("a/2")); err != nil {
			return err
		}
		if !it.Valid() {
			return errors.New("expected valid iterator after seek to a/2")
		}
		if string(it.Key()) != "a/2" {
			return errors.Errorf("expected key a/2 but got %s", string(it.Key()))
		}
		if !it.Next() {
			return errors.New("expected next key after a/2")
		}
		if string(it.Key()) != "a/3" {
			return errors.Errorf("expected key a/3 but got %s", string(it.Key()))
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Test reverse seek with prefix
	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		it := tx.Iterate(ctx, []byte("b/"), true, true)
		defer it.Close()
		if err := it.Seek([]byte("b/2")); err != nil {
			return err
		}
		if !it.Valid() {
			return errors.New("expected valid iterator after reverse seek to b/2")
		}
		if string(it.Key()) != "b/2" {
			return errors.Errorf("expected key b/2 but got %s", string(it.Key()))
		}
		if !it.Next() {
			return errors.New("expected next key after b/2 in reverse")
		}
		if string(it.Key()) != "b/1" {
			return errors.Errorf("expected key b/1 but got %s", string(it.Key()))
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Test seek to nil (should go to start/end based on direction)
	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		// Forward direction
		it := tx.Iterate(ctx, nil, true, false)
		defer it.Close()
		if err := it.Seek(nil); err != nil {
			return err
		}
		if !it.Valid() {
			return errors.New("expected valid iterator after seek to nil (forward)")
		}
		if string(it.Key()) != "a/1" {
			return errors.Errorf("expected first key a/1 but got %s", string(it.Key()))
		}

		// Reverse direction
		it = tx.Iterate(ctx, nil, true, true)
		defer it.Close()
		if err := it.Seek(nil); err != nil {
			return err
		}
		if !it.Valid() {
			return errors.New("expected valid iterator after seek to nil (reverse)")
		}
		if string(it.Key()) != "test-2" {
			return errors.Errorf("expected last key test-2 but got %s", string(it.Key()))
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Test seek with prefix constraint
	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		it := tx.Iterate(ctx, []byte("b/"), true, false)
		defer it.Close()
		if err := it.Seek([]byte("a/3")); err != nil {
			return err
		}
		if !it.Valid() {
			return errors.New("expected valid iterator after seek to a/3 with b/ prefix")
		}
		if string(it.Key()) != "b/1" {
			return errors.Errorf("expected first matching key b/1 but got %s", string(it.Key()))
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Test reverse seek positioning
	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		it := tx.Iterate(ctx, []byte("b/"), true, true)
		defer it.Close()
		if err := it.Seek([]byte("b/1.5")); err != nil {
			return err
		}
		if !it.Valid() {
			return errors.New("expected valid iterator after reverse seek to b/1.5")
		}
		// Should land on b/1 since it's the greatest key <= b/1.5
		if string(it.Key()) != "b/1" {
			return errors.Errorf("expected key b/1 but got %s", string(it.Key()))
		}
		// Moving next in reverse should give us no more keys since b/1 is the smallest in the b/ prefix
		if it.Next() || it.Valid() {
			return errors.Errorf("expected no more valid keys but got %s", string(it.Key()))
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Cleanup test data
	err = withTx(ctx, ktx, true, func(tx kvtx.Tx) error {
		for _, x := range testData {
			if err := tx.Delete(ctx, x.k); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	})
	return err
}

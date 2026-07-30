package kvtx_kvtest

import (
	"bytes"
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// withTx executes a replay-safe function within a transaction context.
func withTx(ctx context.Context, ktx kvtx.Store, writable bool, fn func(tx kvtx.Tx) error) error {
	return kvtx.RunTransaction(ctx, writable,
		func(ctx context.Context) (kvtx.Tx, error) {
			return ktx.NewTransaction(ctx, writable)
		},
		func(_ context.Context, tx kvtx.Tx) error {
			return fn(tx)
		},
	)
}

// withTxValue executes a replay-safe function and returns its value after the
// logical transaction succeeds.
func withTxValue[T any](
	ctx context.Context,
	ktx kvtx.Store,
	writable bool,
	fn func(tx kvtx.Tx) (T, error),
) (T, error) {
	var value T
	err := withTx(ctx, ktx, writable, func(tx kvtx.Tx) error {
		attemptValue, err := fn(tx)
		if err == nil {
			value = attemptValue
		}
		return err
	})
	return value, err
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
		return nil
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
		return nil
	})
	if err != nil {
		return err
	}

	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		_, ok, err := tx.Get(ctx, keys[0])
		if err == nil && ok {
			err = errors.Errorf("expected key to remain deleted: %s", string(keys[0]))
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
		return nil
	})
	if err != nil {
		return err
	}

	ks, err := withTxValue(ctx, ktx, false, func(tx kvtx.Tx) ([][]byte, error) {
		var attemptKeys [][]byte
		err := tx.ScanPrefix(ctx, []byte("t"), func(key, val []byte) error {
			attemptKeys = append(attemptKeys, bytes.Clone(key))
			return nil
		})
		return attemptKeys, err
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
		return nil
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
		return nil
	})
	if err != nil {
		return err
	}

	// GetBatch must return values and found flags aligned with the requested
	// keys, identical to calling Get for each key in the same transaction. A
	// wrapper that transforms keys (prefixer) or serves values from an overlay
	// (txcache) has to keep that alignment through its batch path, including
	// absent keys interleaved with present ones.
	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		batchKeys := [][]byte{
			[]byte("a/1"),
			[]byte("missing/1"),
			[]byte("b/2"),
			[]byte("foo-1"),
			[]byte("missing/2"),
		}
		values, found, err := kvtx.GetBatch(ctx, tx, batchKeys)
		if err != nil {
			return err
		}
		if len(values) != len(batchKeys) || len(found) != len(batchKeys) {
			return errors.Errorf("GetBatch returned %d values %d found for %d keys", len(values), len(found), len(batchKeys))
		}
		for i, k := range batchKeys {
			wantVal, wantFound, err := tx.Get(ctx, k)
			if err != nil {
				return err
			}
			if found[i] != wantFound {
				return errors.Errorf("GetBatch found[%d]=%v for %s, want %v", i, found[i], string(k), wantFound)
			}
			if !bytes.Equal(values[i], wantVal) {
				return errors.Errorf("GetBatch value[%d] mismatch for %s", i, string(k))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// GetBatch must reject an empty key in the batch with ErrEmptyKey, matching
	// Get's single-key contract.
	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		if _, _, err := kvtx.GetBatch(ctx, tx, [][]byte{[]byte("a/1"), {}}); err != kvtx.ErrEmptyKey {
			return errors.Errorf("expected empty key error from GetBatch but got %v", err)
		}
		return nil
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
		if !it.Valid() || string(it.Key()) != "test-1" {
			return errors.Errorf("expected first prefixed key test-1 but got %q", it.Key())
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

	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		it := tx.Iterate(ctx, []byte("test-"), true, true)
		defer it.Close()

		if err := it.Seek(nil); err != nil {
			return err
		}
		expected := []string{"test-2", "test-1"}
		for _, exp := range expected {
			if !it.Valid() || string(it.Key()) != exp {
				return errors.Errorf("expected reverse prefixed key %s but got %q", exp, it.Key())
			}
			it.Next()
		}
		if err := it.Err(); err != nil {
			return err
		}
		if it.Valid() {
			return errors.Errorf("expected reverse prefixed iterator to stop but got %q", it.Key())
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = withTx(ctx, ktx, false, func(tx kvtx.Tx) error {
		for _, reverse := range []bool{false, true} {
			it := tx.Iterate(ctx, []byte("missing/"), true, reverse)
			if err := it.Seek(nil); err != nil {
				it.Close()
				return err
			}
			if it.Valid() {
				err := errors.Errorf("expected missing prefix to be invalid but got %q", it.Key())
				it.Close()
				return err
			}
			if err := it.Err(); err != nil {
				it.Close()
				return err
			}
			it.Close()
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
		return nil
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
		return nil
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
		return nil
	})
	return err
}

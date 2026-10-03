package kvtx_iterator

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	"gotest.tools/v3/assert"
)

type mockOps struct {
	data map[string]string
}

type generationOps struct {
	generation        uint64
	scannedGeneration uint64
}

func (o *generationOps) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if o.generation != o.scannedGeneration {
		return nil, false, kvtx.ErrInvalidSnapshot
	}
	return []byte("value"), true, nil
}

func (o *generationOps) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	o.scannedGeneration = o.generation
	return cb([]byte("key"))
}

func (m *mockOps) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	val, ok := m.data[string(key)]
	if !ok {
		return nil, false, nil
	}
	return []byte(val), true, nil
}

func (m *mockOps) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	for k := range m.data {
		if len(prefix) == 0 || (len(k) >= len(prefix) && k[:len(prefix)] == string(prefix)) {
			if err := cb([]byte(k)); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestIterator(t *testing.T) {
	// Create transaction keys spanning the prefix and traversal cases.
	ctx := context.Background()
	ops := &mockOps{
		data: map[string]string{
			"a":   "1",
			"b":   "2",
			"c":   "3",
			"d/a": "4",
			"d/b": "5",
			"e":   "6",
		},
	}

	// Verify forward iteration visits every transaction key.
	t.Run("no prefix", func(t *testing.T) {
		// Open a forward iterator over the transaction keys.
		it := NewIterator(ctx, ops, nil, true, false)
		defer it.Close()

		// Collect the transaction keys in iterator order.
		var keys []string
		for it.Next() {
			keys = append(keys, string(it.Key()))
		}

		// Verify forward iteration completed with the expected key order.
		assert.NilError(t, it.Err())
		assert.DeepEqual(t, []string{"a", "b", "c", "d/a", "d/b", "e"}, keys)
	})

	// Verify iteration restricts transaction keys to the requested prefix.
	t.Run("with prefix", func(t *testing.T) {
		// Open a forward iterator restricted to the directory prefix.
		it := NewIterator(ctx, ops, []byte("d/"), true, false)
		defer it.Close()

		// Collect the transaction keys matching the directory prefix.
		var keys []string
		for it.Next() {
			keys = append(keys, string(it.Key()))
		}

		// Verify prefix iteration completed with only matching keys.
		assert.NilError(t, it.Err())
		assert.DeepEqual(t, []string{"d/a", "d/b"}, keys)
	})

	// Verify reverse iteration visits transaction keys in descending order.
	t.Run("reverse", func(t *testing.T) {
		// Open a reverse iterator over the transaction keys.
		it := NewIterator(ctx, ops, nil, true, true)
		defer it.Close()

		// Collect the transaction keys in reverse iterator order.
		var keys []string
		for it.Next() {
			keys = append(keys, string(it.Key()))
		}

		// Verify reverse iteration completed with the expected key order.
		assert.NilError(t, it.Err())
		assert.DeepEqual(t, []string{"e", "d/b", "d/a", "c", "b", "a"}, keys)
	})

	// Verify seeks resolve exact keys, following keys, and the end of the tree.
	t.Run("seek", func(t *testing.T) {
		// Open a forward iterator for the transaction seek cases.
		it := NewIterator(ctx, ops, nil, true, false)
		defer it.Close()

		// Seek to an existing transaction key and verify its stored value.
		assert.NilError(t, it.Seek([]byte("c")))
		assert.Equal(t, "c", string(it.Key()))
		val, err := it.Value()
		assert.NilError(t, err)
		assert.Equal(t, "3", string(val))

		// Seek between transaction keys and verify the following key and value.
		assert.NilError(t, it.Seek([]byte("d")))
		assert.Equal(t, "d/a", string(it.Key()))
		val, err = it.Value()
		assert.NilError(t, err)
		assert.Equal(t, "4", string(val))

		// Verify a seek beyond all transaction keys invalidates the iterator.
		assert.NilError(t, it.Seek([]byte("f")))
		assert.Equal(t, it.Valid(), false)
	})
}

func TestIteratorValuePropagatesSnapshotError(t *testing.T) {
	// Open an iterator over a transaction whose snapshot can change.
	ops := &generationOps{generation: 1}
	it := NewIterator(context.Background(), ops, nil, true, false)
	defer it.Close()

	// Advance the iterator before changing the transaction generation.
	assert.Assert(t, it.Next())
	ops.generation++

	// Read the iterator value after its transaction snapshot changes.
	value, err := it.Value()

	// Verify the value read reports the invalid transaction snapshot.
	assert.Assert(t, errors.Is(err, kvtx.ErrInvalidSnapshot),
		"Value() error = %v, want kvtx.ErrInvalidSnapshot", err)
	assert.Assert(t, value == nil)
}

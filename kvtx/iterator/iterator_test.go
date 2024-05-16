package kvtx_iterator

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"
)

type mockOps struct {
	data map[string]string
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

	t.Run("no prefix", func(t *testing.T) {
		it := NewIterator(ctx, ops, nil, true, false)
		defer it.Close()

		var keys []string
		for it.Next() {
			keys = append(keys, string(it.Key()))
		}
		assert.NilError(t, it.Err())
		assert.DeepEqual(t, []string{"a", "b", "c", "d/a", "d/b", "e"}, keys)
	})

	t.Run("with prefix", func(t *testing.T) {
		it := NewIterator(ctx, ops, []byte("d/"), true, false)
		defer it.Close()

		var keys []string
		for it.Next() {
			keys = append(keys, string(it.Key()))
		}
		assert.NilError(t, it.Err())
		assert.DeepEqual(t, []string{"d/a", "d/b"}, keys)
	})

	t.Run("reverse", func(t *testing.T) {
		it := NewIterator(ctx, ops, nil, true, true)
		defer it.Close()

		var keys []string
		for it.Next() {
			keys = append(keys, string(it.Key()))
		}
		assert.NilError(t, it.Err())
		assert.DeepEqual(t, []string{"e", "d/b", "d/a", "c", "b", "a"}, keys)
	})

	t.Run("seek", func(t *testing.T) {
		it := NewIterator(ctx, ops, nil, true, false)
		defer it.Close()

		assert.NilError(t, it.Seek([]byte("c")))
		assert.Equal(t, "c", string(it.Key()))
		val, err := it.Value()
		assert.NilError(t, err)
		assert.Equal(t, "3", string(val))

		assert.NilError(t, it.Seek([]byte("d")))
		assert.Equal(t, "d/a", string(it.Key()))
		val, err = it.Value()
		assert.NilError(t, err)
		assert.Equal(t, "4", string(val))

		assert.NilError(t, it.Seek([]byte("f")))
		assert.Equal(t, it.Valid(), false)
	})
}

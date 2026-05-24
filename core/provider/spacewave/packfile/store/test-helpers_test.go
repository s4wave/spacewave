package store

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

type memIndexCache struct {
	mu      sync.Mutex
	entries map[string][]byte
}

type errorIndexCache struct {
	getErr error
	setErr error
}

type packItem struct {
	h    *hash.Hash
	data []byte
}

func newMemIndexCache() *memIndexCache {
	return &memIndexCache{entries: make(map[string][]byte)}
}

func (c *memIndexCache) Get(_ context.Context, packID string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[packID]
	return bytes.Clone(e), ok, nil
}

func (c *memIndexCache) Set(_ context.Context, packID string, entries []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[packID] = bytes.Clone(entries)
	return nil
}

func (c *errorIndexCache) Get(_ context.Context, _ string) ([]byte, bool, error) {
	return nil, false, c.getErr
}

func (c *errorIndexCache) Set(_ context.Context, _ string, _ []byte) error {
	return c.setErr
}

func buildTestPack(t *testing.T, blocks map[string][]byte) ([]byte, []byte) {
	t.Helper()
	var items []packItem
	for _, data := range blocks {
		h, err := hash.Sum(hash.HashType_HashType_SHA256, data)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, packItem{h: h, data: data})
	}
	return packItems(t, items)
}

func buildTestPackOrdered(t *testing.T, ordered []struct{ Name, Data string }) ([]byte, []byte) {
	t.Helper()
	items := make([]packItem, len(ordered))
	for i, o := range ordered {
		h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte(o.Data))
		if err != nil {
			t.Fatal(err)
		}
		items[i] = packItem{h: h, data: []byte(o.Data)}
	}
	return packItems(t, items)
}

func packItems(t *testing.T, items []packItem) ([]byte, []byte) {
	t.Helper()
	var buf bytes.Buffer
	idx := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, []byte, error) {
		if idx >= len(items) {
			return nil, nil, nil
		}
		e := items[idx]
		idx++
		return e.h, e.data, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), result.BloomFilter
}

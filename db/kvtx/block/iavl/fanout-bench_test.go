package kvtx_block_iavl

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"sort"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

type fanoutBenchTree struct {
	store  *benchBlockStore
	root   *block.BlockRef
	keys   [][]byte
	fanout int
}

type fanoutBenchEntry struct {
	maxKey []byte
	ref    *block.BlockRef
}

type fanoutBenchNode struct {
	leaf bool
	keys [][]byte
	refs []*block.BlockRef
}

func buildFanoutBenchTree(tb testing.TB, keys [][]byte, fanout int) *fanoutBenchTree {
	tb.Helper()

	ctx := context.Background()
	if err := validateFanoutBenchKeys(keys); err != nil {
		tb.Fatal(err)
	}
	store := newBenchBlockStore()
	entries := make([]fanoutBenchEntry, 0, len(keys))
	for i, key := range keys {
		ref, _, err := store.PutBlock(ctx, benchValue(i), nil)
		if err != nil {
			tb.Fatal(err)
		}
		entries = append(entries, fanoutBenchEntry{maxKey: key, ref: ref})
	}

	for len(entries) > 1 {
		next := make([]fanoutBenchEntry, 0, (len(entries)+fanout-1)/fanout)
		for start := 0; start < len(entries); start += fanout {
			end := min(start+fanout, len(entries))
			node := &fanoutBenchNode{leaf: len(entries) == len(keys)}
			node.keys = make([][]byte, end-start)
			node.refs = make([]*block.BlockRef, end-start)
			for i, entry := range entries[start:end] {
				node.keys[i] = entry.maxKey
				node.refs[i] = entry.ref
			}
			ref, err := putFanoutBenchNode(ctx, store, node)
			if err != nil {
				tb.Fatal(err)
			}
			next = append(next, fanoutBenchEntry{
				maxKey: entries[end-1].maxKey,
				ref:    ref,
			})
		}
		entries = next
	}
	store.resetCounts()

	return &fanoutBenchTree{
		store:  store,
		root:   entries[0].ref,
		keys:   keys,
		fanout: fanout,
	}
}

func putFanoutBenchNode(ctx context.Context, store block.StoreOps, node *fanoutBenchNode) (*block.BlockRef, error) {
	data, err := node.marshal()
	if err != nil {
		return nil, err
	}
	ref, _, err := store.PutBlock(ctx, data, &block.PutOpts{Refs: node.refs})
	return ref, err
}

func loadFanoutBenchNode(ctx context.Context, store block.StoreOps, ref *block.BlockRef) (*fanoutBenchNode, error) {
	data, found, err := store.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return unmarshalFanoutBenchNode(data)
}

func fanoutBenchValueRef(ctx context.Context, tree *fanoutBenchTree, key []byte) (*block.BlockRef, bool, error) {
	ref := tree.root
	for {
		node, err := loadFanoutBenchNode(ctx, tree.store, ref)
		if err != nil {
			return nil, false, err
		}
		idx := node.find(key)
		if idx == len(node.keys) {
			return nil, false, nil
		}
		if node.leaf {
			if !bytes.Equal(node.keys[idx], key) {
				return nil, false, nil
			}
			return node.refs[idx], true, nil
		}
		ref = node.refs[idx]
	}
}

func fanoutBenchGet(ctx context.Context, tree *fanoutBenchTree, key []byte) ([]byte, bool, error) {
	ref, found, err := fanoutBenchValueRef(ctx, tree, key)
	if err != nil || !found {
		return nil, found, err
	}
	data, found, err := tree.store.GetBlock(ctx, ref)
	return data, found, err
}

func fanoutBenchScanPrefixKeys(
	ctx context.Context,
	tree *fanoutBenchTree,
	prefix []byte,
	cb func(key []byte) error,
) error {
	return fanoutBenchScanPrefixRefs(ctx, tree, tree.root, prefix, fanoutBenchPrefixEnd(prefix), func(key []byte, _ *block.BlockRef) error {
		return cb(key)
	})
}

func fanoutBenchScanPrefixValues(
	ctx context.Context,
	tree *fanoutBenchTree,
	prefix []byte,
	cb func(key, value []byte) error,
) error {
	return fanoutBenchScanPrefixRefs(ctx, tree, tree.root, prefix, fanoutBenchPrefixEnd(prefix), func(key []byte, ref *block.BlockRef) error {
		value, found, err := tree.store.GetBlock(ctx, ref)
		if err != nil {
			return err
		}
		if !found {
			return block.ErrNotFound
		}
		return cb(key, value)
	})
}

func fanoutBenchScanPrefixRefs(
	ctx context.Context,
	tree *fanoutBenchTree,
	ref *block.BlockRef,
	prefix []byte,
	end []byte,
	cb func(key []byte, ref *block.BlockRef) error,
) error {
	node, err := loadFanoutBenchNode(ctx, tree.store, ref)
	if err != nil {
		return err
	}
	if node.leaf {
		for i, key := range node.keys {
			if bytes.Compare(key, prefix) < 0 {
				continue
			}
			if end != nil && bytes.Compare(key, end) >= 0 {
				break
			}
			if !bytes.HasPrefix(key, prefix) {
				continue
			}
			if err := cb(key, node.refs[i]); err != nil {
				return err
			}
		}
		return nil
	}

	var prevMax []byte
	for i, maxKey := range node.keys {
		if end != nil && prevMax != nil && bytes.Compare(prevMax, end) >= 0 {
			break
		}
		if bytes.Compare(maxKey, prefix) >= 0 {
			if err := fanoutBenchScanPrefixRefs(ctx, tree, node.refs[i], prefix, end, cb); err != nil {
				return err
			}
		}
		prevMax = maxKey
	}
	return nil
}

func fanoutBenchPrefixEnd(prefix []byte) []byte {
	end := append([]byte(nil), prefix...)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] != 0xff {
			end[i]++
			return end[:i+1]
		}
	}
	return nil
}

func fanoutBenchSet(
	ctx context.Context,
	store block.StoreOps,
	ref *block.BlockRef,
	key []byte,
	value []byte,
) (*block.BlockRef, bool, error) {
	node, err := loadFanoutBenchNode(ctx, store, ref)
	if err != nil {
		return nil, false, err
	}
	idx := node.find(key)
	if idx == len(node.keys) {
		return ref, false, nil
	}
	next := &fanoutBenchNode{
		leaf: node.leaf,
		keys: append([][]byte(nil), node.keys...),
		refs: append([]*block.BlockRef(nil), node.refs...),
	}
	if node.leaf {
		if !bytes.Equal(node.keys[idx], key) {
			return ref, false, nil
		}
		valueRef, _, err := store.PutBlock(ctx, value, nil)
		if err != nil {
			return nil, false, err
		}
		next.refs[idx] = valueRef
		nextRef, err := putFanoutBenchNode(ctx, store, next)
		return nextRef, true, err
	}

	nextRef, changed, err := fanoutBenchSet(ctx, store, node.refs[idx], key, value)
	if err != nil || !changed {
		return ref, changed, err
	}
	next.refs[idx] = nextRef
	outRef, err := putFanoutBenchNode(ctx, store, next)
	return outRef, true, err
}

func (n *fanoutBenchNode) find(key []byte) int {
	return sort.Search(len(n.keys), func(i int) bool {
		return bytes.Compare(key, n.keys[i]) <= 0
	})
}

func validateFanoutBenchKeys(keys [][]byte) error {
	if len(keys) == 0 {
		return errors.New("fanout benchmark requires at least one key")
	}
	for i, key := range keys {
		if len(key) == 0 {
			return errors.New("fanout benchmark does not accept empty keys")
		}
		if i == 0 {
			continue
		}
		if bytes.Compare(keys[i-1], key) >= 0 {
			return errors.New("fanout benchmark keys must be strictly sorted")
		}
	}
	return nil
}

func (n *fanoutBenchNode) marshal() ([]byte, error) {
	out := []byte{'f', 'a', 'n', '1'}
	if n.leaf {
		out = append(out, 1)
	} else {
		out = append(out, 0)
	}
	out = binary.AppendUvarint(out, uint64(len(n.keys)))
	for i, key := range n.keys {
		ref, err := n.refs[i].MarshalVT()
		if err != nil {
			return nil, err
		}
		out = binary.AppendUvarint(out, uint64(len(key)))
		out = append(out, key...)
		out = binary.AppendUvarint(out, uint64(len(ref)))
		out = append(out, ref...)
	}
	return out, nil
}

func unmarshalFanoutBenchNode(data []byte) (*fanoutBenchNode, error) {
	if len(data) < 5 || string(data[:4]) != "fan1" {
		return nil, block.ErrUnexpectedType
	}
	reader := bytes.NewReader(data[5:])
	count, err := binary.ReadUvarint(reader)
	if err != nil {
		return nil, err
	}
	node := &fanoutBenchNode{
		leaf: data[4] == 1,
		keys: make([][]byte, count),
		refs: make([]*block.BlockRef, count),
	}
	for i := range node.keys {
		keyLen, err := binary.ReadUvarint(reader)
		if err != nil {
			return nil, err
		}
		key := make([]byte, keyLen)
		if _, err := reader.Read(key); err != nil {
			return nil, err
		}
		refLen, err := binary.ReadUvarint(reader)
		if err != nil {
			return nil, err
		}
		refData := make([]byte, refLen)
		if _, err := reader.Read(refData); err != nil {
			return nil, err
		}
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(refData); err != nil {
			return nil, err
		}
		node.keys[i] = key
		node.refs[i] = ref
	}
	return node, nil
}

func TestFanoutBenchTreeHarness(t *testing.T) {
	ctx := context.Background()
	tree := buildFanoutBenchTree(t, makeBenchKeys(128, benchKeySequential), 32)
	for _, key := range tree.keys {
		_, found, err := fanoutBenchGet(ctx, tree, key)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("key %x not found", key)
		}
	}
	nextRoot, changed, err := fanoutBenchSet(ctx, tree.store, tree.root, tree.keys[17], benchValue(1000))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected update")
	}
	tree.root = nextRoot
	value, found, err := fanoutBenchGet(ctx, tree, tree.keys[17])
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(value, benchValue(1000)) {
		t.Fatal("updated value mismatch")
	}
	for _, key := range [][]byte{
		{0},
		append(append([]byte(nil), tree.keys[len(tree.keys)-1]...), 0),
	} {
		_, found, err := fanoutBenchGet(ctx, tree, key)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			t.Fatalf("unexpected missing-key hit for %x", key)
		}
	}
	_, changed, err = fanoutBenchSet(ctx, tree.store, tree.root, []byte{0xff}, benchValue(1001))
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("unexpected missing-key update")
	}
}

func TestFanoutBenchTreeScanPrefixBoundaries(t *testing.T) {
	ctx := context.Background()
	tree := buildFanoutBenchTree(t, [][]byte{
		[]byte("a/"),
		[]byte("a/0"),
		[]byte("a0"),
		[]byte("aa"),
		[]byte("b"),
	}, 2)

	var keys [][]byte
	if err := fanoutBenchScanPrefixKeys(ctx, tree, []byte("a/"), func(key []byte) error {
		keys = append(keys, append([]byte(nil), key...))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || !bytes.Equal(keys[0], []byte("a/")) || !bytes.Equal(keys[1], []byte("a/0")) {
		t.Fatalf("unexpected a/ prefix keys: %q", keys)
	}
	keyOnlyReads := tree.store.getBlocks.Load()

	tree.store.resetCounts()
	var values int
	if err := fanoutBenchScanPrefixValues(ctx, tree, []byte("a/"), func(key, value []byte) error {
		if !bytes.HasPrefix(key, []byte("a/")) {
			t.Fatalf("key %q crossed prefix boundary", key)
		}
		if len(value) == 0 {
			t.Fatalf("empty value for %q", key)
		}
		values++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if values != 2 {
		t.Fatalf("expected 2 a/ prefix values, got %d", values)
	}
	if tree.store.getBlocks.Load() <= keyOnlyReads {
		t.Fatalf("expected value scan to fetch value blocks, key scan read %d blocks and value scan read %d", keyOnlyReads, tree.store.getBlocks.Load())
	}
}

func TestFanoutBenchTreeScanPrefixGraphAndMissing(t *testing.T) {
	ctx := context.Background()
	tree := buildFanoutBenchTree(t, makeBenchKeys(512, benchKeyGraph), 32)

	var count int
	prefix := benchGraphPrefix(2)
	if err := fanoutBenchScanPrefixKeys(ctx, tree, prefix, func(key []byte) error {
		if !bytes.HasPrefix(key, prefix) {
			t.Fatalf("key %x crossed graph prefix %x", key, prefix)
		}
		count++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != benchGraphGroupSize {
		t.Fatalf("expected %d graph prefix keys, got %d", benchGraphGroupSize, count)
	}

	tree.store.resetCounts()
	count = 0
	if err := fanoutBenchScanPrefixKeys(ctx, tree, benchGraphPrefix(4), func(key []byte) error {
		count++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected missing graph prefix to return 0 keys, got %d", count)
	}
}

func TestFanoutBenchTreeRejectsInvalidKeys(t *testing.T) {
	for _, test := range []struct {
		name string
		keys [][]byte
	}{
		{name: "empty", keys: nil},
		{name: "empty_key", keys: [][]byte{{1}, nil}},
		{name: "duplicate", keys: [][]byte{{1}, {1}}},
		{name: "unsorted", keys: [][]byte{{2}, {1}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateFanoutBenchKeys(test.keys); err == nil {
				t.Fatal("expected invalid keys")
			}
		})
	}
}

func BenchmarkFanoutBlockTreeScanPrefixKeys(b *testing.B) {
	for _, fanout := range []int{16, 32, 64} {
		for _, size := range []int{1024, 16384} {
			b.Run("fanout_"+strconv.Itoa(fanout)+"/"+benchFixtureName(benchKeyGraph, size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildFanoutBenchTree(b, makeBenchKeys(size, benchKeyGraph), fanout)
				tree.store.resetCounts()
				b.ResetTimer()
				for i := range b.N {
					var count int
					err := fanoutBenchScanPrefixKeys(ctx, tree, benchGraphPrefix(benchLookupIndex(i, size)/benchGraphGroupSize), func(key []byte) error {
						count++
						return nil
					})
					if err != nil {
						b.Fatal(err)
					}
					if count == 0 {
						b.Fatal("expected matching prefix keys")
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(tree.fanout), "fanout")
				tree.store.reportMetrics(b, int64(b.N))
			})
		}
	}
}

func BenchmarkFanoutBlockTreeScanPrefixValues(b *testing.B) {
	for _, fanout := range []int{16, 32, 64} {
		for _, size := range []int{1024, 16384} {
			b.Run("fanout_"+strconv.Itoa(fanout)+"/"+benchFixtureName(benchKeyGraph, size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildFanoutBenchTree(b, makeBenchKeys(size, benchKeyGraph), fanout)
				tree.store.resetCounts()
				b.ResetTimer()
				for i := range b.N {
					var count int
					err := fanoutBenchScanPrefixValues(ctx, tree, benchGraphPrefix(benchLookupIndex(i, size)/benchGraphGroupSize), func(_, _ []byte) error {
						count++
						return nil
					})
					if err != nil {
						b.Fatal(err)
					}
					if count == 0 {
						b.Fatal("expected matching prefix values")
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(tree.fanout), "fanout")
				tree.store.reportMetrics(b, int64(b.N))
			})
		}
	}
}

func BenchmarkFanoutBlockTreeGetCursorAtKey(b *testing.B) {
	for _, fanout := range []int{16, 32, 64} {
		for _, size := range []int{1024, 16384} {
			b.Run("fanout_"+strconv.Itoa(fanout)+"/"+benchSizeName(size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildFanoutBenchTree(b, makeBenchKeys(size, benchKeySequential), fanout)
				tree.store.resetCounts()
				b.ResetTimer()
				for i := range b.N {
					_, found, err := fanoutBenchValueRef(ctx, tree, tree.keys[benchLookupIndex(i, size)])
					if err != nil {
						b.Fatal(err)
					}
					if !found {
						b.Fatal("key not found")
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(tree.fanout), "fanout")
				tree.store.reportMetrics(b, int64(b.N))
			})
		}
	}
}

func BenchmarkFanoutBlockTreeGetValue(b *testing.B) {
	for _, fanout := range []int{16, 32, 64} {
		for _, size := range []int{1024, 16384} {
			b.Run("fanout_"+strconv.Itoa(fanout)+"/"+benchSizeName(size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildFanoutBenchTree(b, makeBenchKeys(size, benchKeySequential), fanout)
				tree.store.resetCounts()
				b.ResetTimer()
				for i := range b.N {
					_, found, err := fanoutBenchGet(ctx, tree, tree.keys[benchLookupIndex(i, size)])
					if err != nil {
						b.Fatal(err)
					}
					if !found {
						b.Fatal("key not found")
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(tree.fanout), "fanout")
				tree.store.reportMetrics(b, int64(b.N))
			})
		}
	}
}

func BenchmarkFanoutBlockTreeUpdate(b *testing.B) {
	for _, fanout := range []int{16, 32, 64} {
		for _, size := range []int{1024, 16384} {
			b.Run("updates_100/fanout_"+strconv.Itoa(fanout)+"/"+benchSizeName(size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildFanoutBenchTree(b, makeBenchKeys(size, benchKeySequential), fanout)
				tree.store.resetCounts()
				b.ResetTimer()
				for i := range b.N {
					root := tree.root
					for updateIndex := range 100 {
						nextRoot, changed, err := fanoutBenchSet(
							ctx,
							tree.store,
							root,
							tree.keys[benchLookupIndex(i+updateIndex, size)],
							benchValue(i+updateIndex+size),
						)
						if err != nil {
							b.Fatal(err)
						}
						if !changed {
							b.Fatal("key not found")
						}
						root = nextRoot
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(tree.fanout), "fanout")
				tree.store.reportMetrics(b, int64(b.N))
			})
		}
	}
}

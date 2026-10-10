package store

import (
	"context"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/packfile"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
)

// BenchmarkPackfileStoreColdPackRead reads every block of a cold 4 MiB pack
// of 4 KiB blocks in random order with writeback enabled, and waits until the
// writeback target holds every block.
func BenchmarkPackfileStoreColdPackRead(b *testing.B) {
	// Build deterministic blocks and references for the cold-pack benchmark.
	const blockCount = 1024
	rng := rand.New(rand.NewPCG(1, 2))
	items := make([]packItem, blockCount)
	refs := make([]*block.BlockRef, blockCount)
	for i := range items {
		data := make([]byte, 4<<10)
		for j := range data {
			data[j] = byte(rng.Uint32())
		}
		h, err := hash.Sum(hash.RecommendedHashType, data)
		if err != nil {
			b.Fatal(err)
		}
		items[i] = packItem{h: h, data: data}
		refs[i] = block.NewBlockRef(h)
	}

	// Pack the benchmark blocks and randomize their read order.
	data, bloom := packItems(b, items)
	rng.Shuffle(len(refs), func(i, j int) { refs[i], refs[j] = refs[j], refs[i] })
	transport := &bytesTransport{data: data}

	// Measure cold-pack reads including completion of every block writeback.
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		// Create a fresh store and writeback target for the benchmark iteration.
		store := NewPackfileStore(func(id string, size int64) (*PackReader, error) {
			return NewPackReader(id, size, transport), nil
		}, newMemIndexCache())
		target := &countingStore{
			StoreOps: block_store_inmem.NewInmemBlock(
				store_kvkey.NewDefaultKVKey(),
				store_kvtx_inmem.NewStore(),
				hash.RecommendedHashType,
				false,
			),
			want: blockCount,
			done: make(chan struct{}),
		}

		// Publish the benchmark pack with target-only writeback alignment.
		store.SetWriteback(b.Context(), target, 0)
		store.UpdateManifest([]*packfile.PackfileEntry{{
			Id: "bench", BloomFilter: bloom, BlockCount: blockCount, SizeBytes: uint64(len(data)),
		}})

		// Read every benchmark block and await writeback before closing.
		for _, ref := range refs {
			if _, found, err := store.GetBlock(b.Context(), ref); err != nil || !found {
				b.Fatalf("GetBlock: found=%v err=%v", found, err)
			}
		}
		<-target.done
		store.Close()
	}
}

// countingStore closes done once want distinct blocks were written.
type countingStore struct {
	block.StoreOps
	want int
	done chan struct{}

	mtx  sync.Mutex
	seen map[string]struct{}
}

func (s *countingStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := s.StoreOps.PutBlock(ctx, data, opts)
	if err == nil {
		s.count(ref)
	}
	return ref, existed, err
}

func (s *countingStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	existed, err := s.StoreOps.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		s.count(entry.Ref)
	}
	return existed, nil
}

func (s *countingStore) count(ref *block.BlockRef) {
	// Count distinct written references under the store lock and signal completion.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.seen == nil {
		s.seen = make(map[string]struct{})
	}
	s.seen[ref.MarshalString()] = struct{}{}
	if len(s.seen) == s.want {
		close(s.done)
	}
}

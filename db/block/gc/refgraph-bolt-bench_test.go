//go:build !js && !wasip1

package block_gc

import (
	"path/filepath"
	"strconv"
	"testing"

	store_kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
)

// BenchmarkRefGraphDurableBatch includes the real Bolt commit and fsync path
// used by a native block-store drain. Database setup is outside the sample.
func BenchmarkRefGraphDurableBatch(b *testing.B) {
	ctx := b.Context()
	store, err := store_kvtx_bolt.Open(filepath.Join(b.TempDir(), "refs.db"), 0o600, nil, []byte("test"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.GetDB().Close()
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		b.Fatal(err)
	}
	defer rg.Close()
	edges := make([]RefEdge, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := range b.N {
		b.StopTimer()
		for i := range edges {
			edges[i] = RefEdge{Subject: "batch-" + strconv.Itoa(iteration), Object: "block-" + strconv.Itoa(i)}
		}
		b.StartTimer()
		if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
			b.Fatal(err)
		}
	}
}

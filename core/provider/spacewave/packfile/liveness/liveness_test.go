package liveness

import (
	"bytes"
	"context"
	"io"
	"testing"

	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/net/hash"
)

func testPack(t *testing.T, blocks ...[]byte) ([]byte, []*hash.Hash) {
	t.Helper()
	hashes := make([]*hash.Hash, 0, len(blocks))
	var buf bytes.Buffer
	idx := 0
	_, err := writer.PackBlocks(&buf, func() (*hash.Hash, []byte, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		data := blocks[idx]
		h, err := hash.Sum(hash.HashType_HashType_SHA256, data)
		if err != nil {
			return nil, nil, err
		}
		hashes = append(hashes, h)
		idx++
		return h, data, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), hashes
}

func TestAnalyzeClassifiesPackLiveness(t *testing.T) {
	ctx := context.Background()
	liveBytes, liveHashes := testPack(t, []byte("live-a"), []byte("live-b"))
	partialBytes, partialHashes := testPack(t, []byte("partial-a"), []byte("partial-b"))
	deadBytes, _ := testPack(t, []byte("dead-a"), []byte("dead-b"))
	packs := map[string][]byte{
		"live":    liveBytes,
		"partial": partialBytes,
		"dead":    deadBytes,
	}
	entries := []*packfile.PackfileEntry{
		{Id: "live", BlockCount: 2, SizeBytes: uint64(len(liveBytes))},
		{Id: "partial", BlockCount: 2, SizeBytes: uint64(len(partialBytes))},
		{Id: "dead", BlockCount: 2, SizeBytes: uint64(len(deadBytes))},
		{Id: "new", BlockCount: 0},
	}
	graph := newTestRefGraph()
	for _, h := range liveHashes {
		graph.addLiveHash(h)
	}
	graph.addLiveHash(partialHashes[0])

	report, err := Analyze(ctx, entries, graph, func(
		_ context.Context,
		entry *packfile.PackfileEntry,
	) (io.ReaderAt, error) {
		return bytes.NewReader(packs[entry.GetId()]), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.PacksScanned != 4 {
		t.Fatalf("PacksScanned = %d, want 4", report.PacksScanned)
	}
	if report.CandidatePacks != 2 {
		t.Fatalf("CandidatePacks = %d, want 2", report.CandidatePacks)
	}
	if report.FullyDeadBytes != uint64(len(deadBytes)) {
		t.Fatalf("FullyDeadBytes = %d, want %d", report.FullyDeadBytes, len(deadBytes))
	}
	if report.PartiallyDeadBytes != uint64(len(partialBytes)) {
		t.Fatalf("PartiallyDeadBytes = %d, want %d", report.PartiallyDeadBytes, len(partialBytes))
	}
	wantCounts := map[PackState]uint64{
		PackStateLive:          1,
		PackStatePartiallyDead: 1,
		PackStateFullyDead:     1,
		PackStateNewlyWritten:  1,
	}
	gotCounts := map[PackState]uint64{
		PackStateLive:          report.LivePacks,
		PackStatePartiallyDead: report.PartiallyDeadPacks,
		PackStateFullyDead:     report.FullyDeadPacks,
		PackStateNewlyWritten:  report.NewlyWrittenPacks,
	}
	for state, want := range wantCounts {
		if gotCounts[state] != want {
			t.Fatalf("%s count = %d, want %d", state, gotCounts[state], want)
		}
	}
	states := make(map[string]PackState, len(report.Packs))
	for _, pack := range report.Packs {
		states[pack.Entry.GetId()] = pack.State
	}
	for id, want := range map[string]PackState{
		"live":    PackStateLive,
		"partial": PackStatePartiallyDead,
		"dead":    PackStateFullyDead,
		"new":     PackStateNewlyWritten,
	} {
		if states[id] != want {
			t.Fatalf("pack %s state = %s, want %s", id, states[id], want)
		}
	}
}

type testRefGraph struct {
	live map[string]struct{}
}

func newTestRefGraph() *testRefGraph {
	return &testRefGraph{live: make(map[string]struct{})}
}

func (g *testRefGraph) addLiveHash(h *hash.Hash) {
	g.live[block_gc.BlockIRI(block.NewBlockRef(h))] = struct{}{}
}

func (g *testRefGraph) AddRef(context.Context, string, string) error    { return nil }
func (g *testRefGraph) RemoveRef(context.Context, string, string) error { return nil }
func (g *testRefGraph) ApplyRefBatch(context.Context, []block_gc.RefEdge, []block_gc.RefEdge) error {
	return nil
}
func (g *testRefGraph) RemoveNodeRefs(context.Context, string, bool) ([]string, error) {
	return nil, nil
}
func (g *testRefGraph) HasIncomingRefs(_ context.Context, node string) (bool, error) {
	_, ok := g.live[node]
	return ok, nil
}
func (g *testRefGraph) HasIncomingRefsExcluding(context.Context, string, ...string) (bool, error) {
	return false, nil
}
func (g *testRefGraph) GetOutgoingRefs(context.Context, string) ([]string, error) { return nil, nil }
func (g *testRefGraph) GetIncomingRefs(context.Context, string) ([]string, error) { return nil, nil }
func (g *testRefGraph) GetUnreferencedNodes(context.Context) ([]string, error)    { return nil, nil }
func (g *testRefGraph) AddBlockRef(context.Context, *block.BlockRef, *block.BlockRef) error {
	return nil
}
func (g *testRefGraph) AddObjectRoot(context.Context, string, *block.BlockRef) error {
	return nil
}
func (g *testRefGraph) RemoveObjectRoot(context.Context, string, *block.BlockRef) error {
	return nil
}
func (g *testRefGraph) Close() error { return nil }

var _ block_gc.RefGraphOps = (*testRefGraph)(nil)

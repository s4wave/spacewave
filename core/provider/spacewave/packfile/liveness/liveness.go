package liveness

import (
	"context"
	"io"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/net/hash"
)

// ReaderAtFunc opens immutable pack bytes for an entry.
type ReaderAtFunc func(ctx context.Context, entry *packfile.PackfileEntry) (io.ReaderAt, error)

// PackState classifies one packfile against the current RefGraph.
type PackState string

const (
	PackStateNewlyWritten  PackState = "newly-written"
	PackStateLive          PackState = "live"
	PackStatePartiallyDead PackState = "partially-dead"
	PackStateFullyDead     PackState = "fully-dead"
)

// PackLiveness is the read-only reachability summary for one manifest entry.
type PackLiveness struct {
	Entry             *packfile.PackfileEntry
	State             PackState
	IndexedBlocks     uint64
	ReachableBlocks   uint64
	UnreachableBlocks uint64
}

// Report summarizes packfile reachability without deleting pack bytes.
type Report struct {
	PacksScanned       uint64
	CandidatePacks     uint64
	LivePacks          uint64
	PartiallyDeadPacks uint64
	FullyDeadPacks     uint64
	NewlyWrittenPacks  uint64
	FullyDeadBytes     uint64
	PartiallyDeadBytes uint64
	Packs              []*PackLiveness
}

// Analyze classifies packfiles by scanning their indexes and checking each
// indexed block against the GC RefGraph.
func Analyze(
	ctx context.Context,
	entries []*packfile.PackfileEntry,
	graph block_gc.RefGraphOps,
	open ReaderAtFunc,
) (*Report, error) {
	if graph == nil {
		return nil, errors.New("ref graph is nil")
	}
	if open == nil {
		return nil, errors.New("pack reader opener is nil")
	}
	report := &Report{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		live, err := analyzeEntry(ctx, entry, graph, open)
		if err != nil {
			return nil, errors.Wrapf(err, "analyze pack %s", entry.GetId())
		}
		report.PacksScanned++
		report.Packs = append(report.Packs, live)
		switch live.State {
		case PackStateNewlyWritten:
			report.NewlyWrittenPacks++
		case PackStateLive:
			report.LivePacks++
		case PackStatePartiallyDead:
			report.CandidatePacks++
			report.PartiallyDeadPacks++
			report.PartiallyDeadBytes += live.Entry.GetSizeBytes()
		case PackStateFullyDead:
			report.CandidatePacks++
			report.FullyDeadPacks++
			report.FullyDeadBytes += live.Entry.GetSizeBytes()
		}
	}
	return report, nil
}

func analyzeEntry(
	ctx context.Context,
	entry *packfile.PackfileEntry,
	graph block_gc.RefGraphOps,
	open ReaderAtFunc,
) (*PackLiveness, error) {
	if entry.GetId() == "" {
		return nil, errors.New("pack id is empty")
	}
	if entry.GetBlockCount() == 0 {
		return &PackLiveness{Entry: entry, State: PackStateNewlyWritten}, nil
	}
	size := entry.GetSizeBytes()
	if size == 0 {
		return nil, errors.New("pack size is empty")
	}
	ra, err := open(ctx, entry)
	if err != nil {
		return nil, err
	}
	reader, err := kvfile.BuildReader(ra, size)
	if err != nil {
		return nil, errors.Wrap(err, "build kvfile reader")
	}

	out := &PackLiveness{Entry: entry}
	err = reader.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		h := &hash.Hash{}
		if err := h.ParseFromB58(string(ie.GetKey())); err != nil {
			return errors.Wrap(err, "parse block hash key")
		}
		has, err := graph.HasIncomingRefs(ctx, block_gc.BlockIRI(block.NewBlockRef(h)))
		if err != nil {
			return err
		}
		out.IndexedBlocks++
		if has {
			out.ReachableBlocks++
		} else {
			out.UnreachableBlocks++
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "scan pack index")
	}
	if out.IndexedBlocks == 0 {
		out.State = PackStateNewlyWritten
		return out, nil
	}
	if out.ReachableBlocks == 0 {
		out.State = PackStateFullyDead
		return out, nil
	}
	if out.UnreachableBlocks != 0 {
		out.State = PackStatePartiallyDead
		return out, nil
	}
	out.State = PackStateLive
	return out, nil
}

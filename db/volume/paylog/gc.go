package paylog

import (
	"context"
	"slices"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume/journal"
)

// Append journals reference graph changes for the collector in one ordered
// index commit. The durable commit that publishes the blocks they name makes
// the entry durable too. It holds the stop-the-world lock shared.
func (s *Store) Append(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	s.stw.RLock()
	defer s.stw.RUnlock()
	return s.AppendJournalOrdered(ctx, adds, removes)
}

// AcquireSTW waits for the running journal appends and excludes new ones
// until release is called.
func (s *Store) AcquireSTW(context.Context) (func(), error) {
	s.stw.Lock()
	return s.stw.Unlock, nil
}

// GetPendingOutgoingRefs returns the targets of journaled edges from node,
// net of journaled removals, in journal order.
func (s *Store) GetPendingOutgoingRefs(ctx context.Context, node string) ([]string, error) {
	// Net the journaled adds and removes from node in order.
	var targets []string
	err := s.view(ctx, func(tx kvtx.Tx) error {
		return tx.ScanPrefix(ctx, []byte(journalPrefix), func(_, value []byte) error {
			// Decode the entry, then apply its edges from node.
			adds, removes, err := journal.Unmarshal(value)
			if err != nil {
				return err
			}
			for _, edge := range adds {
				if edge.Subject == node {
					targets = append(targets, edge.Object)
				}
			}
			for _, edge := range removes {
				if edge.Subject == node {
					targets = slices.DeleteFunc(targets, func(target string) bool { return target == edge.Object })
				}
			}
			return nil
		})
	})
	return targets, err
}

// ReplayWAL applies the journal to graph, removes the applied entries, and
// returns how many it applied.
func (s *Store) ReplayWAL(ctx context.Context, graph block_gc.CollectorGraph) (int, error) {
	var n int
	err := s.ReplayJournal(ctx, func(adds, removes []block_gc.RefEdge) error {
		n++
		return graph.ApplyRefBatch(ctx, adds, removes)
	})
	return n, err
}

// _ is a type assertion
var _ block_gc.WALAppender = (*Store)(nil)

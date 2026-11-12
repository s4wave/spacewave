package world_block

import (
	"context"

	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/tx"
)

// NewGraphSettings builds the settings holding indexes. The default indexes
// are the empty settings.
func NewGraphSettings(indexes []cayley_kv.QuadIndex) (*GraphSettings, error) {
	if len(indexes) == 0 || cayley_kv.CompareQuadIndexes(indexes, cayley_kv.DefaultQuadIndexes) {
		return nil, nil
	}
	metas, err := cayley_kv.QuadIndexMetas(indexes)
	if err != nil {
		return nil, err
	}
	return &GraphSettings{Indexes: metas}, nil
}

// QuadIndexes returns the quad indexes with their states, or nil for the
// default indexes.
func (s *GraphSettings) QuadIndexes() ([]cayley_kv.QuadIndex, error) {
	if len(s.GetIndexes()) == 0 {
		return nil, nil
	}
	return cayley_kv.QuadIndexesFromMetas(s.GetIndexes())
}

// ValidateInitial checks the settings a new World may start with: every index
// must be complete, since a World starts with no change in progress.
func (s *GraphSettings) ValidateInitial() error {
	indexes, err := s.QuadIndexes()
	if err != nil {
		return err
	}
	for _, index := range indexes {
		if len(index.Dirs) == 0 {
			return errors.New("graph settings: index has no directions")
		}
		if index.State != cayley_kv.QuadIndexState_COMPLETE {
			return errors.Errorf("graph settings: initial index is %s", index.State)
		}
	}
	return nil
}

// graphOptions returns the options that open the graph store under the
// settings without reading them from the store.
func (s *GraphSettings) graphOptions() (graph.Options, error) {
	indexes, err := s.QuadIndexes()
	if err != nil {
		return nil, err
	}
	if indexes == nil {
		// The default indexes: the store reads no layout.
		return graph.Options{cayley_kv.OptAssumeDefaultIdx: true}, nil
	}
	return graph.Options{cayley_kv.OptTrustQuadIndexes: indexes}, nil
}

// StepGraphSettings advances the World's graph toward the target settings by
// at most max links. It reports whether the graph matches target and whether
// this call changed the World. Expects rmtx to be locked.
//
// The first call records the change in the graph store and later calls
// continue it; a different change in progress is an error. Each call is a
// function of the World and its arguments only, so every replay of the same
// calls reaches the same World.
func (t *WorldState) StepGraphSettings(ctx context.Context, target *GraphSettings, max int) (done, changed bool, err error) {
	// Reject a read-only state and an invalid target.
	if !t.write {
		return false, false, tx.ErrNotWrite
	}
	if max < 1 {
		return false, false, errors.New("graph settings step max must be positive")
	}
	if err := target.ValidateInitial(); err != nil {
		return false, false, err
	}
	indexes, err := target.QuadIndexes()
	if err != nil {
		return false, false, err
	}

	// A node graph has fixed edge ranges: it already matches the defaults and
	// takes no other settings.
	if t.format == WorldFormat_WORLD_FORMAT_NODE_GRAPH {
		if indexes != nil {
			return false, false, errors.New("node graph World takes no graph settings")
		}
		return true, false, nil
	}
	if indexes == nil {
		indexes = cayley_kv.DefaultQuadIndexes
	}

	// Skip a graph that already matches.
	graphHd, err := t.graph(ctx)
	if err != nil {
		return false, false, err
	}
	qs, ok := graphHd.QuadStore.(*cayley_kv.QuadStore)
	if !ok {
		return false, false, errors.New("world graph is not a cayley kv store")
	}
	if cayley_kv.CompareQuadIndexes(qs.Layout(), indexes) {
		return true, false, nil
	}

	// Begin or continue the change and advance it one step.
	if err := qs.BeginIndexChange(ctx, indexes); err != nil {
		return false, false, err
	}
	done, err = qs.StepIndexChange(ctx, max)
	if err != nil {
		return false, false, err
	}

	// Record the graph's new layout in the root, which opens it next time.
	r, err := t.GetRoot(ctx)
	if err != nil {
		return false, false, err
	}
	r.GraphSettings, err = NewGraphSettings(qs.Layout())
	if err != nil {
		return false, false, err
	}
	t.bcs.MarkDirty()
	return done, true, nil
}

// StepGraphSettings advances the transaction's World toward the target graph
// settings by at most max links, as WorldState.StepGraphSettings does. The
// caller commits the transaction when changed is set.
func (e *EngineTx) StepGraphSettings(ctx context.Context, target *GraphSettings, max int) (done, changed bool, err error) {
	// Step the write transaction's World under its lock.
	if e.writeTx == nil {
		return false, false, tx.ErrNotWrite
	}
	err = e.writeTx.locked(ctx, true, func() error {
		done, changed, err = e.writeTx.state.StepGraphSettings(ctx, target, max)
		return err
	})
	return done, changed, err
}

// GetGraphSettings returns the graph settings of the transaction's World, nil
// for the default settings.
func (t *Tx) GetGraphSettings(ctx context.Context) (*GraphSettings, error) {
	// Read the root under the transaction's read lock.
	return lockedValue(ctx, t, false, func() (*GraphSettings, error) {
		r, err := t.state.GetRoot(ctx)
		if err != nil {
			return nil, err
		}
		return r.GetGraphSettings().CloneVT(), nil
	})
}

// GetGraphSettings returns the graph settings of the transaction's World, nil
// for the default settings.
func (e *EngineTx) GetGraphSettings(ctx context.Context) (*GraphSettings, error) {
	// Read through the transaction's retained state.
	return performOpValue(ctx, e, func(tx *Tx) (*GraphSettings, error) {
		return tx.GetGraphSettings(ctx)
	})
}

// DefaultGraphSettingsStepMax bounds the links one graph settings step indexes
// or the postings it deletes, keeping each step's commit short next to
// writers. Measured at 124 ms for the slowest step of a 200,000 link build.
const DefaultGraphSettingsStepMax = 2048

// GraphSettingsStepper is a World engine that changes its graph settings one
// step at a time: a local Engine or a Space World engine.
type GraphSettingsStepper interface {
	// StepGraphSettings advances the World toward target by at most max links
	// and reports whether the World matches target.
	StepGraphSettings(ctx context.Context, target *GraphSettings, max int) (bool, error)
}

// ApplyGraphSettings steps the World to target until it matches, returning the
// number of steps that changed it. A change stopped by ctx or an error resumes
// from the stored progress on the next call.
func ApplyGraphSettings(ctx context.Context, s GraphSettingsStepper, target *GraphSettings, max int) (int, error) {
	for steps := 0; ; steps++ {
		done, err := s.StepGraphSettings(ctx, target, max)
		if err != nil || done {
			return steps, err
		}
	}
}

// _ is a type assertion
var _ GraphSettingsStepper = (*Engine)(nil)

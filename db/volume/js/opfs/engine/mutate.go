package engine

import (
	"bytes"
	"context"
	"sort"
)

// mutate prepares engine-owned point updates against the current locked root.
// The callback may change root accounting and stage bounded immutable files.
func (e *Engine) mutate(ctx context.Context, fn func(*snapshot, *publication) ([]*Record, error)) error {
	// Protect the generation's files and take the publish lock.
	release, err := e.protect(ctx)
	if err != nil {
		return err
	}
	defer release()
	unlock, err := e.backend.Lock(ctx, "publish", true)
	if err != nil {
		return err
	}
	defer unlock()

	// Load the durable root and run the callback against a snapshot.
	root, err := e.loadRoot(ctx)
	if err != nil {
		return err
	}
	p := newPublication(e, root)
	records, err := fn(&snapshot{engine: e, root: root}, p)
	if err != nil {
		return err
	}

	// Skip the commit when the callback changed nothing.
	if len(records) == 0 && len(p.output) == 0 && len(p.retired) == 0 {
		return nil
	}

	// Sort and validate the callback's records before publishing them.
	sort.Slice(records, func(i, j int) bool { return bytes.Compare(records[i].Key, records[j].Key) < 0 })
	if err := validateRecords(records); err != nil {
		return err
	}

	// Rewrite the catalogue paths containing the records.
	if len(records) != 0 {
		children, err := p.updateCatalogue(ctx, root.Catalogue, records)
		if err != nil {
			return err
		}
		p.root.Catalogue, err = p.finishCatalogue(children)
		if err != nil {
			return err
		}
	}

	// Commit the next generation.
	return p.commit(ctx)
}

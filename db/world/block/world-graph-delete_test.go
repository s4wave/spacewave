package world_block

import (
	"context"
	"maps"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

// TestDeleteGraphQuadRevisions checks committed endpoint revisions and absent deletes.
func TestDeleteGraphQuadRevisions(t *testing.T) {
	for _, target := range []string{"b", "a"} {
		t.Run(target, func(t *testing.T) {
			// Create endpoints and distinct objects whose keys contain IRI delimiters.
			ctx := t.Context()
			engine := newRetirementTestEngine(t, ctx)
			writer, err := engine.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Discard()
			keys := []string{"a", "b", "<a>", "<b>"}
			for _, key := range keys {
				obj, err := writer.CreateObject(ctx, key, nil)
				world.ReleaseObjectState(obj)
				if err != nil {
					t.Fatal(err)
				}
			}

			// Commit one relationship and capture its endpoint revisions.
			q := world.NewGraphQuadWithKeys("a", "<next>", target, "")
			if err := writer.SetGraphQuad(ctx, q); err != nil {
				t.Fatal(err)
			}
			if err := writer.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			before, seqno := graphDeleteSnapshot(t, ctx, engine, keys)

			// Delete the relationship through a new transaction and publish the change.
			deleteGraphQuadAndCommit(t, ctx, engine, q)
			after, deletedSeqno := graphDeleteSnapshot(t, ctx, engine, keys)
			want := maps.Clone(before)
			want["a"]++
			want[target]++
			if !maps.Equal(after, want) {
				t.Errorf("deleted relationship revisions = %v, want %v", after, want)
			}
			if deletedSeqno != seqno+1 {
				t.Errorf("deleted relationship sequence = %d, want %d", deletedSeqno, seqno+1)
			}

			// Repeat the missing deletion without changing revisions or the changelog.
			deleteGraphQuadAndCommit(t, ctx, engine, q)
			repeated, repeatedSeqno := graphDeleteSnapshot(t, ctx, engine, keys)
			if !maps.Equal(repeated, after) {
				t.Errorf("missing relationship changed revisions: before=%v after=%v", after, repeated)
			}
			if repeatedSeqno != deletedSeqno {
				t.Errorf("missing relationship changed sequence: before=%d after=%d", deletedSeqno, repeatedSeqno)
			}
		})
	}
}

// graphDeleteSnapshot reads committed object revisions and the World sequence.
func graphDeleteSnapshot(t *testing.T, ctx context.Context, engine *Engine, keys []string) (map[string]uint64, uint64) {
	// Hold a fresh snapshot while reading every requested object.
	t.Helper()
	reader, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Discard()
	revisions := make(map[string]uint64, len(keys))
	for _, key := range keys {
		obj, err := world.MustGetObject(ctx, reader, key)
		if err != nil {
			t.Fatal(err)
		}
		_, revision, err := obj.GetRootRef(ctx)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		revisions[key] = revision
	}

	// Read the sequence from the same immutable snapshot.
	seqno, err := reader.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return revisions, seqno
}

// deleteGraphQuadAndCommit publishes one requested relationship deletion.
func deleteGraphQuadAndCommit(t *testing.T, ctx context.Context, engine *Engine, q world.GraphQuad) {
	// Open a write transaction that owns the graph mutation.
	t.Helper()
	writer, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Discard()

	// Commit both the graph and its endpoint revisions together.
	if err := writer.DeleteGraphQuad(ctx, q); err != nil {
		t.Fatal(err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

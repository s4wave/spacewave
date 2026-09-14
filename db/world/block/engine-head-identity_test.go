package world_block

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
)

// TestEngineUnchangedHeadDoesNotReadBlocks checks that an identical durable
// root does not reopen World state to compare it with itself.
func TestEngineUnchangedHeadDoesNotReadBlocks(t *testing.T) {
	for _, watch := range []bool{false, true} {
		name := "transaction-refresh"
		if watch {
			name = "watch-adoption"
		}
		t.Run(name, func(t *testing.T) {
			// Publish a nonempty World before measuring an unchanged head.
			var durable *bucket.ObjectRef
			engine := newRetirementTestEngine(t, t.Context(), WithWriteCoordinator(
				nil, coord.Scope{}, nil,
				func(context.Context) (*bucket.ObjectRef, error) { return durable.Clone(), nil },
			))
			writer, err := engine.NewTransaction(t.Context(), true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(writer.Discard)
			{
				createdObject, err := writer.CreateObject(t.Context(), "identity/example", nil)
				world.ReleaseObjectState(createdObject)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Commit(t.Context()); err != nil {
				t.Fatal(err)
			}
			durable = engine.GetRootRef()
			if durable.GetRootRef().GetEmpty() {
				t.Fatal("published World root is empty")
			}

			// Exercise the public refresh or watch path with the same DAG identity.
			ctx, counter := block.WithReadCounter(t.Context())
			if watch {
				err = engine.AdoptRootRefFromWatch(ctx, durable.Clone())
			} else {
				reader, readErr := engine.NewTransaction(ctx, false)
				err = readErr
				if reader != nil {
					reader.Discard()
				}
			}
			if err != nil {
				t.Fatal(err)
			}

			// An identical content address requires neither storage nor decoding.
			counts := counter.Snapshot()
			if counts.BlockReadCount != 0 || counts.DecodedBlockCacheAttemptCount != 0 {
				t.Fatalf("unchanged root reopened block state: %+v", counts)
			}
			if !engine.GetRootRef().EqualsRef(durable) {
				t.Fatal("unchanged root update replaced the published identity")
			}
		})
	}
}

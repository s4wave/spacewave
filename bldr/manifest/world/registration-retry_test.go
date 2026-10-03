package bldr_manifest_world

import (
	"context"
	"testing"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
)

// TestManifestRegistrationRetriesInvalidSnapshot checks complete registration
// replay against the real World engine after opening, body, or commit failure.
func TestManifestRegistrationRetriesInvalidSnapshot(t *testing.T) {
	for _, stage := range []string{"open", "body", "commit"} {
		t.Run(stage, func(t *testing.T) {
			// Start a World testbed and create the manifest store.
			ctx := t.Context()
			tb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tb.Release)
			ws := world.NewEngineWorldState(tb.Engine, true)
			const hostKey = "plugin-host"
			if _, err := CreateManifestStore(ctx, ws, hostKey); err != nil {
				t.Fatal(err)
			}

			// Open a stage held until the test ends.
			contentStage, err := ws.StageWorldState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(contentStage.Release)

			// Write a manifest content block through the stage.
			ref, err := world.AccessObject(ctx, contentStage.AccessWorldState, nil, func(cursor *block.Cursor) error {
				cursor.SetBlock(block_mock.NewExample("manifest-content"), true)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			// Record the host's root ref.
			host, err := world.MustGetObject(ctx, ws, hostKey)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { world.ReleaseObjectState(host) })
			_, before, err := host.GetRootRef(ctx)
			if err != nil {
				t.Fatal(err)
			}

			// Fail once after real mutations, before their transaction commits.
			engine := &registrationRetryEngine{Engine: tb.Engine, stage: stage}
			retrying := world.NewEngineWorldState(engine, true)
			meta := &manifest.ManifestMeta{ManifestId: "spacewave-core", BuildType: "release", PlatformId: "js", Rev: 7}
			key := manifest.NewManifestKey(hostKey, meta)
			if err := ExStoreManifestOp(ctx, retrying, "", key, []string{hostKey}, manifest.NewManifestRef(meta, ref)); err != nil {
				t.Fatal(err)
			}

			// The registration must retry and open exactly two write attempts.
			if engine.attempts != 2 {
				t.Fatalf("registration opened %d write attempts, want two", engine.attempts)
			}

			// The registered manifest must hold the original content ref.
			stored, err := world.MustGetObject(ctx, ws, key)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { world.ReleaseObjectState(stored) })
			actual, _, err := stored.GetRootRef(ctx)
			if err != nil || !actual.EqualVT(ref) {
				t.Fatalf("registered manifest reference: %v, error: %v", actual, err)
			}

			// The registration must link the manifest to the host exactly once.
			quads, err := ws.LookupGraphQuads(ctx, NewManifestQuad(hostKey, key, meta.GetManifestId()), 0)
			if err != nil || len(quads) != 1 {
				t.Fatalf("registration links: %v, error: %v", quads, err)
			}

			// The graph insertion and explicit registration notification each
			// advance the host once; discarded attempts must add neither.
			_, after, err := host.GetRootRef(ctx)
			if err != nil || after != before+2 {
				t.Fatalf("host revision: before=%d after=%d error=%v", before, after, err)
			}
		})
	}
}

// registrationRetryEngine injects one snapshot failure around real transactions.
type registrationRetryEngine struct {
	world.Engine
	stage    string
	attempts int
}

// NewTransaction opens the real transaction except for an injected open failure.
func (e *registrationRetryEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	// Count write attempts and fail the first open when the stage is set.
	if write {
		e.attempts++
		if e.attempts == 1 && e.stage == "open" {
			return nil, kvtx.ErrInvalidSnapshot
		}
	}

	// Open the real transaction from the wrapped engine.
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	if !write || e.attempts != 1 {
		return tx, nil
	}
	return &registrationRetryTx{Tx: tx, stage: e.stage}, nil
}

// registrationRetryTx leaves failed mutations uncommitted for the owner to discard.
type registrationRetryTx struct {
	world.Tx
	stage string
}

// ApplyWorldOp runs the real operation before injecting the body failure.
func (t *registrationRetryTx) ApplyWorldOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	seq, sysErr, err := t.Tx.ApplyWorldOp(ctx, op, sender)
	if err == nil && t.stage == "body" {
		err = kvtx.ErrInvalidSnapshot
	}
	return seq, sysErr, err
}

// Commit injects failure before the real transaction publishes its mutations.
func (t *registrationRetryTx) Commit(ctx context.Context) error {
	if t.stage == "commit" {
		return kvtx.ErrInvalidSnapshot
	}
	return t.Tx.Commit(ctx)
}

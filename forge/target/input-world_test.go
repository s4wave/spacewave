package forge_target

import (
	"testing"

	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestInputWorldRetainsSelectedIdentity exercises both engine resolution paths.
func TestInputWorldRetainsSelectedIdentity(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		t.Run(map[bool]string{false: "deferred", true: "immediate"}[immediate], func(t *testing.T) {
			// Resolve the registered engine through the production input contract.
			tb := world_testbed.MustDefault(t, t.Context())
			input := &InputWorld{EngineId: tb.EngineID, LookupImmediate: immediate}
			value, release, err := input.ResolveValue(t.Context(), tb.Bus)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(release)

			// Verify the selected ID accompanies an observable transactional capability.
			if value.GetWorldEngineID() != tb.EngineID {
				t.Fatalf("engine identity = %q, want %q", value.GetWorldEngineID(), tb.EngineID)
			}
			if err := value.Validate(); err != nil {
				t.Fatal(err)
			}
			tx, err := value.GetWorldEngine().NewTransaction(t.Context(), false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tx.Discard)
			if _, err := tx.GetSeqno(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestInputWorldEmptyIdentity distinguishes empty values from a transactional grant.
func TestInputWorldEmptyIdentity(t *testing.T) {
	// Empty and nontransactional inputs carry no invented registry identity.
	tb := world_testbed.MustDefault(t, t.Context())
	for _, value := range []InputValueWorld{
		NewInputValueWorld("", nil, nil),
		NewInputValueWorld("", nil, tb.WorldState),
	} {
		if value.GetWorldEngineID() != "" {
			t.Fatal("empty identity was reconstructed")
		}
		if err := value.Validate(); err != nil {
			t.Fatal(err)
		}
	}

	// A transactional grant requires its selected ID before it can be borrowed.
	value := NewInputValueWorld("", tb.Engine, tb.WorldState)
	if err := value.Validate(); err != world.ErrEmptyEngineID {
		t.Fatalf("unidentified transactional input = %v, want %v", err, world.ErrEmptyEngineID)
	}
}

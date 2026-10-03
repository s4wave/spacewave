package block_gc

import (
	"context"
	"reflect"
	"strconv"
	"testing"
)

// TestResolveIRIRefKeysDoesNotRetainResolvedIRIs pins that resolving names does
// not add history-sized state to a RefGraph. Cayley owns the durable name index;
// each operation should retain only its bounded result set.
func TestResolveIRIRefKeysDoesNotRetainResolvedIRIs(t *testing.T) {
	// Open the reference graph for resident IRI state checks.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Seed distinct IRI pairs into the reference graph.
	const iriCount = 128
	edges := make([]RefEdge, iriCount)
	for i := range edges {
		edges[i] = RefEdge{
			Subject: "resident-state/subject/" + strconv.Itoa(i),
			Object:  "resident-state/object/" + strconv.Itoa(i),
		}
	}
	if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
		t.Fatal(err)
	}

	// Resolve each seeded IRI and check retained graph state.
	for i, edge := range edges {
		// Resolve the next distinct IRI to operation-local keys.
		keys, err := rg.resolveIRIRefKeys(ctx, []string{edge.Subject})
		if err != nil {
			t.Fatal(err)
		}

		// Verify resolution returns one key without retaining IRI history.
		if len(keys) != 1 {
			t.Fatalf("resolving %q returned %d ref keys, want 1", edge.Subject, len(keys))
		}
		if got := residentIRIRefKeyCount(rg); got != 0 {
			t.Fatalf("resident IRI ref-key state grew to %d after resolving %d distinct IRIs", got, i+1)
		}
	}
}

func residentIRIRefKeyCount(rg *RefGraph) int {
	field := reflect.ValueOf(rg).Elem().FieldByName("iriRefKeys")
	if !field.IsValid() {
		return 0
	}
	return field.Len()
}

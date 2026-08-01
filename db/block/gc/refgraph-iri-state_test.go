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
	ctx := context.Background()
	rg := newTestRefGraph(t)

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

	for i, edge := range edges {
		keys, err := rg.resolveIRIRefKeys(ctx, []string{edge.Subject})
		if err != nil {
			t.Fatal(err)
		}
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

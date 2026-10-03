package block_gc

import (
	"context"
	"testing"
)

func TestRegisterEntityChain_TwoNodes(t *testing.T) {
	// Open the reference graph for entity chain registration.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Register the two-node entity chain.
	if err := RegisterEntityChain(ctx, rg, "a", "b"); err != nil {
		t.Fatal(err)
	}

	// Read the first entity node outgoing edges.
	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the registered chain retains its expected next node.
	if len(refs) != 1 || refs[0] != "b" {
		t.Fatalf("expected [b], got %v", refs)
	}
}

func TestRegisterEntityChain_ThreeNodes(t *testing.T) {
	// Open the reference graph for entity chain registration.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Register the three-node entity chain.
	if err := RegisterEntityChain(ctx, rg, "a", "b", "c"); err != nil {
		t.Fatal(err)
	}

	// Read the first entity node outgoing edges.
	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the registered chain retains its expected next node.
	if len(refs) != 1 || refs[0] != "b" {
		t.Fatalf("expected a->[b], got %v", refs)
	}

	// Read the second entity node outgoing edges.
	refs, err = rg.GetOutgoingRefs(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the registered chain retains its expected next node.
	if len(refs) != 1 || refs[0] != "c" {
		t.Fatalf("expected b->[c], got %v", refs)
	}
}

func TestRegisterEntityChain_TooFewNodes(t *testing.T) {
	// Open the reference graph for entity chain registration.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Verify registration rejects a single entity node.
	err := RegisterEntityChain(ctx, rg, "a")
	if err == nil {
		t.Fatal("expected error for single node")
	}

	// Verify registration rejects an empty entity chain.
	err = RegisterEntityChain(ctx, rg)
	if err == nil {
		t.Fatal("expected error for zero nodes")
	}
}

func TestRegisterEntityChain_Idempotent(t *testing.T) {
	// Open the reference graph for entity chain registration.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Register the three-node entity chain.
	if err := RegisterEntityChain(ctx, rg, "a", "b", "c"); err != nil {
		t.Fatal(err)
	}

	// Repeat registration to verify the chain remains unchanged.
	if err := RegisterEntityChain(ctx, rg, "a", "b", "c"); err != nil {
		t.Fatal(err)
	}

	// Read the first entity node outgoing edges.
	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the registered chain retains its expected next node.
	if len(refs) != 1 || refs[0] != "b" {
		t.Fatalf("expected [b] after idempotent call, got %v", refs)
	}
}

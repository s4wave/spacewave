package block_gc

import (
	"context"
	"testing"
)

func TestRegisterEntityChain_TwoNodes(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	if err := RegisterEntityChain(ctx, rg, "a", "b"); err != nil {
		t.Fatal(err)
	}

	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != "b" {
		t.Fatalf("expected [b], got %v", refs)
	}
}

func TestRegisterEntityChain_ThreeNodes(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	if err := RegisterEntityChain(ctx, rg, "a", "b", "c"); err != nil {
		t.Fatal(err)
	}

	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != "b" {
		t.Fatalf("expected a->[b], got %v", refs)
	}

	refs, err = rg.GetOutgoingRefs(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != "c" {
		t.Fatalf("expected b->[c], got %v", refs)
	}
}

func TestRegisterEntityChain_TooFewNodes(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	err := RegisterEntityChain(ctx, rg, "a")
	if err == nil {
		t.Fatal("expected error for single node")
	}

	err = RegisterEntityChain(ctx, rg)
	if err == nil {
		t.Fatal("expected error for zero nodes")
	}
}

func TestRegisterEntityChain_Idempotent(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	if err := RegisterEntityChain(ctx, rg, "a", "b", "c"); err != nil {
		t.Fatal(err)
	}
	if err := RegisterEntityChain(ctx, rg, "a", "b", "c"); err != nil {
		t.Fatal(err)
	}

	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != "b" {
		t.Fatalf("expected [b] after idempotent call, got %v", refs)
	}
}

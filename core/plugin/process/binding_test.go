package process_binding

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
)

func TestProcessBindingWriteRetriesRealTransaction(t *testing.T) {
	ctx := context.Background()
	store := kvtest.NewFaultStore(
		hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()),
		kvtest.FaultBeforeCommit,
	)
	binding := &s4wave_process.ProcessBinding{
		State:     s4wave_process.ProcessBindingState_ProcessBindingState_APPROVED,
		ObjectKey: "object-key",
		TypeId:    "type-id",
	}

	if err := SetProcessBinding(ctx, store, "space-id", binding.GetObjectKey(), binding); err != nil {
		t.Fatal(err)
	}
	if got := store.Opened(); got != 2 {
		t.Fatalf("opened transactions = %d, want 2", got)
	}
	if got := store.Discarded(); got != 2 {
		t.Fatalf("discarded transactions = %d, want 2", got)
	}
	if got := store.DelegatedCommits(); got != 1 {
		t.Fatalf("delegated commits = %d, want 1", got)
	}

	got, err := GetProcessBinding(ctx, store, "space-id", binding.GetObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if got.GetObjectKey() != binding.GetObjectKey() || got.GetTypeId() != binding.GetTypeId() || got.GetState() != binding.GetState() {
		t.Fatalf("binding = %v, want %v", got, binding)
	}
	list, err := ListProcessBindings(ctx, store, "space-id")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].GetObjectKey() != binding.GetObjectKey() {
		t.Fatalf("bindings = %v, want one %v", list, binding)
	}
}

func TestDeleteProcessBindingKeepsNewerDecision(t *testing.T) {
	ctx := context.Background()
	store := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	stale := &s4wave_process.ProcessBinding{
		State:     s4wave_process.ProcessBindingState_ProcessBindingState_APPROVED,
		ObjectKey: "object-key",
		TypeId:    "type-id",
	}
	newer := stale.CloneVT()
	newer.State = s4wave_process.ProcessBindingState_ProcessBindingState_UNAPPROVED
	if err := SetProcessBinding(ctx, store, "space-id", newer.GetObjectKey(), newer); err != nil {
		t.Fatal(err)
	}

	// Deleting a binding the store no longer holds keeps the newer decision.
	if err := DeleteProcessBinding(ctx, store, "space-id", stale); err != nil {
		t.Fatal(err)
	}
	got, err := GetProcessBinding(ctx, store, "space-id", newer.GetObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if !got.EqualVT(newer) {
		t.Fatalf("binding = %v, want %v", got, newer)
	}

	if err := DeleteProcessBinding(ctx, store, "space-id", newer); err != nil {
		t.Fatal(err)
	}
	got, err = GetProcessBinding(ctx, store, "space-id", newer.GetObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("binding = %v after delete, want none", got)
	}
}

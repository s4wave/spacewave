package session_controller

import (
	"testing"

	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

func TestTransitionSessionPreservesIndexAndMetadata(t *testing.T) {
	ctx := t.Context()
	controller := &Controller{objStore: store_kvtx_inmem.NewStore()}
	ref := func(account string) *session.SessionRef {
		return &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{ProviderId: "local", ProviderAccountId: account, Id: "same-session"}}
	}
	source, target, unrelated := ref("source"), ref("target"), ref("unrelated")
	old, err := controller.RegisterSession(ctx, source, &session.SessionMetadata{DisplayName: "My laptop", CreatedAt: 1700000000000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.RegisterSession(ctx, target, nil); err != nil {
		t.Fatal(err)
	}
	other, err := controller.RegisterSession(ctx, unrelated, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := controller.TransitionSession(ctx, source, target); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := controller.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d Sessions, want 2", len(entries))
	}
	for _, entry := range entries {
		switch entry.GetSessionIndex() {
		case old.GetSessionIndex():
			if !entry.GetSessionRef().EqualVT(target) {
				t.Fatal("source index did not move")
			}
		case other.GetSessionIndex():
			if !entry.GetSessionRef().EqualVT(unrelated) {
				t.Fatal("unrelated Session changed")
			}
		default:
			t.Fatal("duplicate target Session remains")
		}
	}
	metadata, err := controller.GetSessionMetadata(ctx, old.GetSessionIndex())
	if err != nil {
		t.Fatal(err)
	}
	if metadata.GetDisplayName() != "My laptop" || metadata.GetCreatedAt() != 1700000000000 || metadata.GetProviderAccountId() != "target" || metadata.GetProviderDisplayName() != "Local" {
		t.Fatalf("metadata changed unexpectedly: %v", metadata)
	}
}

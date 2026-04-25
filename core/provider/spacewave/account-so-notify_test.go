package provider_spacewave

import (
	"context"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
)

func TestHandleAccountSONotifyMetadataAppliesCaches(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	meta, err := space.NewSharedObjectMeta("Old Space")
	if err != nil {
		t.Fatalf("build old metadata: %v", err)
	}
	acc.cacheSharedObjectListEntry(&sobject.SharedObjectListEntry{
		Ref:    acc.buildSharedObjectRef("so-1"),
		Meta:   meta,
		Source: "cloud",
	})

	acc.handleAccountSONotify("so-1", &api.SONotifyEventPayload{
		ChangeType: "metadata",
		Metadata: &api.SpaceMetadataResponse{
			OwnerType:   sobject.OwnerTypeAccount,
			OwnerId:     "test-account",
			DisplayName: "New Space",
			ObjectType:  "space",
			PublicRead:  true,
		},
	})

	metadata, err := acc.GetSharedObjectMetadata(context.Background(), "so-1")
	if err != nil {
		t.Fatalf("get metadata: %v", err)
	}
	if metadata.GetDisplayName() != "New Space" || !metadata.GetPublicRead() {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
	list := acc.soListCtr.GetValue()
	if list == nil || len(list.GetSharedObjects()) != 1 {
		t.Fatalf("expected cached shared object list entry, got %#v", list)
	}
	if got := getSharedObjectDisplayName(list.GetSharedObjects()[0].GetMeta()); got != "New Space" {
		t.Fatalf("unexpected patched list display name: %q", got)
	}
}

func TestHandleAccountSONotifyDeleteRemovesCaches(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	meta, err := space.NewSharedObjectMeta("Deleted Space")
	if err != nil {
		t.Fatalf("build old metadata: %v", err)
	}
	acc.cacheSharedObjectListEntry(&sobject.SharedObjectListEntry{
		Ref:    acc.buildSharedObjectRef("so-1"),
		Meta:   meta,
		Source: "cloud",
	})
	acc.SetSharedObjectMetadata("so-1", &api.SpaceMetadataResponse{
		OwnerType:   sobject.OwnerTypeAccount,
		OwnerId:     "test-account",
		DisplayName: "Deleted Space",
		ObjectType:  "space",
	})

	acc.handleAccountSONotify("so-1", &api.SONotifyEventPayload{
		ChangeType: "delete",
	})

	if _, err := acc.GetSharedObjectMetadata(context.Background(), "so-1"); err != ErrSharedObjectMetadataDeleted {
		t.Fatalf("expected deleted metadata tombstone, got %v", err)
	}
	list := acc.soListCtr.GetValue()
	if list == nil || len(list.GetSharedObjects()) != 0 {
		t.Fatalf("expected deleted shared object removed from list cache, got %#v", list)
	}
}

func TestHandleAccountSONotifyStateEventKeepsKnownCaches(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	meta, err := space.NewSharedObjectMeta("Known Space")
	if err != nil {
		t.Fatalf("build shared object metadata: %v", err)
	}
	acc.cacheSharedObjectListEntry(&sobject.SharedObjectListEntry{
		Ref:    acc.buildSharedObjectRef("so-1"),
		Meta:   meta,
		Source: "cloud",
	})
	acc.SetSharedObjectMetadata("so-1", &api.SpaceMetadataResponse{
		OwnerType:   sobject.OwnerTypeAccount,
		OwnerId:     "test-account",
		DisplayName: "Known Space",
		ObjectType:  "space",
	})

	acc.handleAccountSONotify("so-1", &api.SONotifyEventPayload{
		ChangeType: "op",
	})

	metadata, err := acc.GetSharedObjectMetadata(context.Background(), "so-1")
	if err != nil {
		t.Fatalf("get metadata: %v", err)
	}
	if metadata.GetDisplayName() != "Known Space" {
		t.Fatalf("unexpected metadata display name: %q", metadata.GetDisplayName())
	}
	list := acc.soListCtr.GetValue()
	if list == nil || len(list.GetSharedObjects()) != 1 {
		t.Fatalf("expected known shared object to stay cached, got %#v", list)
	}
	if got := getSharedObjectDisplayName(list.GetSharedObjects()[0].GetMeta()); got != "Known Space" {
		t.Fatalf("unexpected list display name: %q", got)
	}
}

func TestHandleAccountSONotifyUnknownInvalidatesList(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	var invalidated int
	acc.soListBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.soListInvalidate = func() {
			invalidated++
		}
	})

	acc.handleAccountSONotify("so-unknown", &api.SONotifyEventPayload{
		ChangeType: "op",
	})

	if invalidated != 1 {
		t.Fatalf("expected one list invalidation, got %d", invalidated)
	}
}

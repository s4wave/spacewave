package provider_spacewave

import (
	"context"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
)

// TestHandleAccountSONotifyMetadataAppliesCaches patches the metadata cache and
// the list from a metadata event.
func TestHandleAccountSONotifyMetadataAppliesCaches(t *testing.T) {
	// Cache a list entry with the old name.
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

	// Deliver a metadata event with the new name.
	acc.handleAccountSONotify(context.Background(), "so-1", &api.SONotifyEventPayload{
		ChangeType: "metadata",
		Metadata: &api.SpaceMetadataResponse{
			OwnerType:   sobject.OwnerTypeAccount,
			OwnerId:     "test-account",
			DisplayName: "New Space",
			ObjectType:  "space",
			PublicRead:  true,
		},
	})

	// Both caches carry the new name.
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
	if got := space.SharedObjectName(list.GetSharedObjects()[0].GetMeta()); got != "New Space" {
		t.Fatalf("unexpected patched list display name: %q", got)
	}
}

// TestHandleAccountSONotifyDeleteRemovesCaches drops a deleted object from both
// caches.
func TestHandleAccountSONotifyDeleteRemovesCaches(t *testing.T) {
	// Cache the object's list entry and metadata.
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

	// Deliver a delete event.
	acc.handleAccountSONotify(context.Background(), "so-1", &api.SONotifyEventPayload{
		ChangeType: "delete",
	})

	// The metadata becomes a tombstone and the list entry leaves.
	if _, err := acc.GetSharedObjectMetadata(context.Background(), "so-1"); err != ErrSharedObjectMetadataDeleted {
		t.Fatalf("expected deleted metadata tombstone, got %v", err)
	}
	list := acc.soListCtr.GetValue()
	if list == nil || len(list.GetSharedObjects()) != 0 {
		t.Fatalf("expected deleted shared object removed from list cache, got %#v", list)
	}
}

// TestHandleAccountSONotifyStateEventKeepsKnownCaches leaves both caches alone
// on a state event for a known object.
func TestHandleAccountSONotifyStateEventKeepsKnownCaches(t *testing.T) {
	// Cache the object's list entry and metadata.
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

	// Deliver a state event.
	acc.handleAccountSONotify(context.Background(), "so-1", &api.SONotifyEventPayload{
		ChangeType: "op",
	})

	// Both caches still hold the object.
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
	if got := space.SharedObjectName(list.GetSharedObjects()[0].GetMeta()); got != "Known Space" {
		t.Fatalf("unexpected list display name: %q", got)
	}
}

// TestHandleAccountSONotifyUnknownInvalidatesList refetches the list for an
// event about an unknown object.
func TestHandleAccountSONotifyUnknownInvalidatesList(t *testing.T) {
	// Count list invalidations.
	acc := NewTestProviderAccount(t, "http://example.invalid")
	var invalidated int
	acc.soListBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.soListInvalidate = func() bool {
			invalidated++
			return true
		}
	})

	// An event about an unknown object invalidates the list once.
	acc.handleAccountSONotify(context.Background(), "so-unknown", &api.SONotifyEventPayload{
		ChangeType: "op",
	})
	if invalidated != 1 {
		t.Fatalf("expected one list invalidation, got %d", invalidated)
	}
}

// TestHandleAccountSONotifyAccessChangedInvalidatesList refetches the list when
// access to a known object changes.
func TestHandleAccountSONotifyAccessChangedInvalidatesList(t *testing.T) {
	// Cache the object's list entry.
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

	// Count list invalidations.
	var invalidated int
	acc.soListBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.soListInvalidate = func() bool {
			invalidated++
			return true
		}
	})

	// An access change invalidates the list once.
	acc.handleAccountSONotify(context.Background(), "so-1", &api.SONotifyEventPayload{
		ChangeType: "access_changed",
	})
	if invalidated != 1 {
		t.Fatalf("expected one list invalidation, got %d", invalidated)
	}
}

// TestHandleAccountSONotifyAccessChangedSettlesCreatedEntry lets the cloud list
// decide membership of a created object after an access change.
func TestHandleAccountSONotifyAccessChangedSettlesCreatedEntry(t *testing.T) {
	// Cache an entry this account created.
	acc := NewTestProviderAccount(t, "http://example.invalid")
	meta, err := space.NewSharedObjectMeta("Created Space")
	if err != nil {
		t.Fatalf("build shared object metadata: %v", err)
	}
	acc.cacheSharedObjectListEntry(&sobject.SharedObjectListEntry{
		Ref:    acc.buildSharedObjectRef("so-1"),
		Meta:   meta,
		Source: "created",
	})

	// Revoke access to it.
	acc.handleAccountSONotify(context.Background(), "so-1", &api.SONotifyEventPayload{
		ChangeType: "access_changed",
	})

	// A cloud snapshot without the object now drops it.
	merged := mergeSharedObjectListSnapshot(&sobject.SharedObjectList{}, acc.soListCtr.GetValue())
	if n := len(merged.GetSharedObjects()); n != 0 {
		t.Fatalf("expected the created entry to leave with the snapshot, got %d entries", n)
	}
}

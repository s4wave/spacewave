package mailboxcache

import (
	"context"
	"errors"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestStoreSyncsPendingMailboxEntries(t *testing.T) {
	store := NewStore()
	fetcher := &testFetcher{resp: &api.GetMailboxResponse{
		Entries: []*api.MailboxEntry{{
			Id:        7,
			InviteId:  "inv-1",
			PeerId:    "peer-1",
			Status:    "pending",
			CreatedAt: 123,
			AccountId: "acct-1",
			EntityId:  "alice",
		}},
	}}
	changed, err := store.Sync(context.Background(), "so-1", true, fetcher, nil)
	if err != nil {
		t.Fatalf("sync mailbox entries: %v", err)
	}
	if !changed {
		t.Fatal("expected sync to change cache")
	}

	entries, valid := store.EntriesSnapshot("so-1")
	if !valid {
		t.Fatal("expected mailbox snapshot to be valid")
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 mailbox entry, got %d", len(entries))
	}
	if entries[0].GetInviteId() != "inv-1" || entries[0].GetPeerId() != "peer-1" {
		t.Fatalf("unexpected mailbox entry: %+v", entries[0])
	}
	if entries[0].GetAccountId() != "acct-1" || entries[0].GetEntityId() != "alice" {
		t.Fatalf("unexpected mailbox identity: %+v", entries[0])
	}
}

func TestStoreSyncAccessDeniedSetsEmptySnapshot(t *testing.T) {
	wantErr := errors.New("denied")
	store := NewStore()
	changed, err := store.Sync(
		context.Background(),
		"so-1",
		true,
		&testFetcher{err: wantErr},
		func(err error) bool { return errors.Is(err, wantErr) },
	)
	if err != nil {
		t.Fatalf("sync mailbox entries: %v", err)
	}
	if !changed {
		t.Fatal("expected empty access-denied snapshot to change cache")
	}
	entries, valid := store.EntriesSnapshot("so-1")
	if !valid {
		t.Fatal("expected mailbox snapshot to be valid")
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty mailbox entries, got %d", len(entries))
	}
}

func TestStoreSyncReadOnlySkipsFetch(t *testing.T) {
	store := NewStore()
	fetcher := &testFetcher{err: errors.New("unexpected fetch")}
	entriesChanged, err := store.Sync(context.Background(), "so-1", false, fetcher, nil)
	if err != nil {
		t.Fatalf("sync readonly mailbox entries: %v", err)
	}
	if !entriesChanged {
		t.Fatal("expected readonly sync to seed empty cache")
	}
	if fetcher.calls != 0 {
		t.Fatalf("expected readonly sync to skip fetch, got %d calls", fetcher.calls)
	}
	entries, valid := store.EntriesSnapshot("so-1")
	if !valid {
		t.Fatal("expected mailbox snapshot to be valid")
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty mailbox entries, got %d", len(entries))
	}
}

func TestStoreRemoveEntry(t *testing.T) {
	store := NewStore()
	store.SetResponse("so-1", &api.GetMailboxResponse{
		Entries: []*api.MailboxEntry{{
			Id:        9,
			InviteId:  "inv-9",
			PeerId:    "peer-9",
			Status:    "pending",
			CreatedAt: 99,
		}},
	})

	if !store.RemoveEntry("so-1", 9) {
		t.Fatal("expected remove to change cache")
	}

	entries, valid := store.EntriesSnapshot("so-1")
	if !valid {
		t.Fatal("expected mailbox snapshot to be valid")
	}
	if len(entries) != 0 {
		t.Fatalf("expected mailbox entry to be removed, got %d entries", len(entries))
	}
	resp, valid := store.ResponseSnapshot("so-1")
	if !valid {
		t.Fatal("expected response snapshot to be valid")
	}
	if len(resp.GetEntries()) != 0 {
		t.Fatalf("expected response entry to be removed, got %d entries", len(resp.GetEntries()))
	}
}

func TestStoreApplyEventPreservesIdentity(t *testing.T) {
	store := NewStore()
	_, changed := store.ApplyEvent("so-1", &api.MailboxEntry{
		Id:        11,
		InviteId:  "inv-11",
		PeerId:    "peer-11",
		Status:    "pending",
		CreatedAt: 111,
		AccountId: "acct-11",
		EntityId:  "casey",
	}, 1)
	if !changed {
		t.Fatal("expected event to change cache")
	}

	entries, valid := store.EntriesSnapshot("so-1")
	if !valid {
		t.Fatal("expected mailbox snapshot to be valid")
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 mailbox entry, got %d", len(entries))
	}
	if entries[0].GetAccountId() != "acct-11" || entries[0].GetEntityId() != "casey" {
		t.Fatalf("unexpected mailbox identity: %+v", entries[0])
	}
}

func TestStoreApplyTerminalEventTracksRequest(t *testing.T) {
	store := NewStore()
	store.SetResponse("so-1", &api.GetMailboxResponse{
		Entries: []*api.MailboxEntry{{
			Id:       11,
			InviteId: "inv-11",
			PeerId:   "peer-11",
			Status:   "pending",
		}},
	})
	track, changed := store.ApplyEvent("so-1", &api.MailboxEntry{
		Id:       11,
		InviteId: "inv-11",
		PeerId:   "peer-11",
		Status:   "accepted",
	}, 2)
	if !changed {
		t.Fatal("expected terminal event to change cache")
	}
	if track.InviteID != "inv-11" || track.PeerID != "peer-11" || track.Status != "accepted" {
		t.Fatalf("unexpected track event: %+v", track)
	}
	entries, valid := store.EntriesSnapshot("so-1")
	if !valid {
		t.Fatal("expected mailbox snapshot to be valid")
	}
	if len(entries) != 0 {
		t.Fatalf("expected terminal event to remove pending entry, got %d", len(entries))
	}
}

type testFetcher struct {
	resp  *api.GetMailboxResponse
	err   error
	calls int
}

func (f *testFetcher) GetMailboxEntries(context.Context, string) (*api.GetMailboxResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.resp.CloneVT(), nil
}

package mailboxcache

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// Fetcher fetches pending mailbox entries for one shared object.
type Fetcher interface {
	GetMailboxEntries(ctx context.Context, soID string) (*api.GetMailboxResponse, error)
}

// AccessDenied reports whether a fetch error should produce an empty valid
// cache instead of failing the sync.
type AccessDenied func(error) bool

// TrackEvent describes a terminal mailbox entry mirrored to request tracking.
type TrackEvent struct {
	InviteID string
	PeerID   string
	Status   string
}

// Store caches owner-visible pending mailbox metadata by shared object id.
type Store struct {
	mut    sync.Mutex
	states map[string]*state
}

type state struct {
	response     *api.GetMailboxResponse
	entries      []*s4wave_provider_spacewave.MailboxEntryInfo
	valid        bool
	entryVersion map[int64]int64
}

// NewStore builds a pending mailbox cache store.
func NewStore() *Store {
	return &Store{}
}

// EntriesSnapshot returns the cached pending mailbox entries for an SO.
func (s *Store) EntriesSnapshot(
	soID string,
) ([]*s4wave_provider_spacewave.MailboxEntryInfo, bool) {
	if s == nil {
		return nil, false
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	state := s.states[soID]
	if state == nil {
		return nil, false
	}
	return cloneEntries(state.entries), state.valid
}

// ResponseSnapshot returns the cached full pending mailbox response for an SO
// when it is available and valid.
func (s *Store) ResponseSnapshot(soID string) (*api.GetMailboxResponse, bool) {
	if s == nil {
		return nil, false
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	state := s.states[soID]
	if state == nil || !state.valid || state.response == nil {
		return nil, false
	}
	return state.response.CloneVT(), true
}

// Sync fetches and stores pending mailbox metadata for an SO.
func (s *Store) Sync(
	ctx context.Context,
	soID string,
	canAccess bool,
	fetcher Fetcher,
	accessDenied AccessDenied,
) (bool, error) {
	if !canAccess {
		return s.SetResponse(soID, &api.GetMailboxResponse{}), nil
	}
	if fetcher == nil {
		return false, errors.New("session client not ready")
	}

	resp, err := fetcher.GetMailboxEntries(ctx, soID)
	if err != nil {
		if accessDenied != nil && accessDenied(err) {
			return s.SetResponse(soID, &api.GetMailboxResponse{}), nil
		}
		return false, err
	}
	return s.SetResponse(soID, resp), nil
}

// SetResponse stores the full pending mailbox cache for an SO.
func (s *Store) SetResponse(soID string, resp *api.GetMailboxResponse) bool {
	if s == nil || soID == "" {
		return false
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	state := s.getOrCreateLocked(soID)
	nextResp := cloneResponse(resp)
	nextEntries := entriesToInfo(nextResp)
	if state.valid &&
		entriesEqual(state.entries, nextEntries) &&
		responseEqual(state.response, nextResp) {
		return false
	}
	state.response = nextResp
	state.entries = nextEntries
	state.valid = true
	return true
}

// ApplyEvent merges a mailbox entry event into the pending cache.
func (s *Store) ApplyEvent(
	soID string,
	entry *api.MailboxEntry,
	updatedAt int64,
) (TrackEvent, bool) {
	if s == nil || soID == "" || entry == nil {
		return TrackEvent{}, false
	}
	id := entry.GetId()
	if id == 0 {
		return TrackEvent{}, false
	}
	info := entryToInfo(entry)
	var changed bool
	s.mut.Lock()
	state := s.getOrCreateLocked(soID)
	if state.entryVersion == nil {
		state.entryVersion = make(map[int64]int64)
	}
	if last, ok := state.entryVersion[id]; ok && updatedAt != 0 && updatedAt <= last {
		s.mut.Unlock()
		return TrackEvent{}, false
	}
	state.entryVersion[id] = updatedAt
	nextEntries, entriesChanged := upsertEntry(state.entries, info)
	nextResp, respChanged := upsertResponse(state.response, entry)
	if !entriesChanged && !respChanged {
		if !state.valid {
			state.entries = nextEntries
			state.response = nextResp
			state.valid = true
			changed = true
		}
	} else {
		state.entries = nextEntries
		state.response = nextResp
		state.valid = true
		changed = true
	}
	s.mut.Unlock()

	if status := entry.GetStatus(); status != "" && status != "pending" {
		return TrackEvent{
			InviteID: entry.GetInviteId(),
			PeerID:   entry.GetPeerId(),
			Status:   status,
		}, changed
	}
	return TrackEvent{}, changed
}

// InvalidateAll marks every cached pending mailbox entry as stale.
func (s *Store) InvalidateAll() bool {
	if s == nil {
		return false
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	if len(s.states) == 0 {
		return false
	}
	for _, state := range s.states {
		if state == nil {
			continue
		}
		state.valid = false
		state.entryVersion = nil
	}
	return true
}

// RemoveEntry removes a processed mailbox entry from the cached pending set.
func (s *Store) RemoveEntry(soID string, entryID int64) bool {
	if s == nil {
		return false
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	state := s.getOrCreateLocked(soID)
	if len(state.entries) == 0 && (state.response == nil || len(state.response.GetEntries()) == 0) {
		return false
	}

	next := make([]*s4wave_provider_spacewave.MailboxEntryInfo, 0, len(state.entries))
	nextResp := &api.GetMailboxResponse{}
	if state.response != nil {
		nextResp = state.response.CloneVT()
	}
	changed := false
	for _, entry := range state.entries {
		if entry.GetId() == entryID {
			changed = true
			continue
		}
		next = append(next, cloneEntry(entry))
	}
	if state.response != nil {
		filtered := make([]*api.MailboxEntry, 0, len(state.response.GetEntries()))
		for _, entry := range state.response.GetEntries() {
			if entry.GetId() == entryID {
				changed = true
				continue
			}
			filtered = append(filtered, entry.CloneVT())
		}
		nextResp.Entries = filtered
	}
	if !changed {
		return false
	}
	state.entries = next
	state.response = nextResp
	state.valid = true
	return true
}

func (s *Store) getOrCreateLocked(soID string) *state {
	if s.states == nil {
		s.states = make(map[string]*state)
	}
	st := s.states[soID]
	if st == nil {
		st = &state{}
		s.states[soID] = st
	}
	return st
}

func entriesToInfo(resp *api.GetMailboxResponse) []*s4wave_provider_spacewave.MailboxEntryInfo {
	if resp == nil {
		return nil
	}
	entries := make([]*s4wave_provider_spacewave.MailboxEntryInfo, 0, len(resp.GetEntries()))
	for _, entry := range resp.GetEntries() {
		if entry.GetStatus() != "pending" {
			continue
		}
		entries = append(entries, entryToInfo(entry))
	}
	return entries
}

func entryToInfo(entry *api.MailboxEntry) *s4wave_provider_spacewave.MailboxEntryInfo {
	if entry == nil {
		return nil
	}
	return &s4wave_provider_spacewave.MailboxEntryInfo{
		Id:        entry.GetId(),
		InviteId:  entry.GetInviteId(),
		PeerId:    entry.GetPeerId(),
		Status:    entry.GetStatus(),
		CreatedAt: entry.GetCreatedAt(),
		AccountId: entry.GetAccountId(),
		EntityId:  entry.GetEntityId(),
	}
}

func upsertEntry(
	current []*s4wave_provider_spacewave.MailboxEntryInfo,
	info *s4wave_provider_spacewave.MailboxEntryInfo,
) ([]*s4wave_provider_spacewave.MailboxEntryInfo, bool) {
	id := info.GetId()
	terminal := info.GetStatus() != "" && info.GetStatus() != "pending"
	found := -1
	for i, e := range current {
		if e.GetId() == id {
			found = i
			break
		}
	}
	if terminal {
		if found < 0 {
			return current, false
		}
		next := make([]*s4wave_provider_spacewave.MailboxEntryInfo, 0, len(current)-1)
		for i, e := range current {
			if i == found {
				continue
			}
			next = append(next, cloneEntry(e))
		}
		return next, true
	}
	if found >= 0 {
		existing := current[found]
		if existing.GetInviteId() == info.GetInviteId() &&
			existing.GetPeerId() == info.GetPeerId() &&
			existing.GetStatus() == info.GetStatus() &&
			existing.GetCreatedAt() == info.GetCreatedAt() {
			return current, false
		}
		next := make([]*s4wave_provider_spacewave.MailboxEntryInfo, len(current))
		for i, e := range current {
			if i == found {
				next[i] = cloneEntry(info)
				continue
			}
			next[i] = cloneEntry(e)
		}
		return next, true
	}
	next := make([]*s4wave_provider_spacewave.MailboxEntryInfo, 0, len(current)+1)
	for _, e := range current {
		next = append(next, cloneEntry(e))
	}
	next = append(next, cloneEntry(info))
	return next, true
}

func upsertResponse(
	current *api.GetMailboxResponse,
	entry *api.MailboxEntry,
) (*api.GetMailboxResponse, bool) {
	if entry == nil {
		return current, false
	}
	next := &api.GetMailboxResponse{}
	if current != nil {
		next = current.CloneVT()
	}
	id := entry.GetId()
	terminal := entry.GetStatus() != "" && entry.GetStatus() != "pending"
	found := -1
	for i, candidate := range next.GetEntries() {
		if candidate.GetId() == id {
			found = i
			break
		}
	}
	if terminal {
		if found < 0 {
			return next, false
		}
		filtered := make([]*api.MailboxEntry, 0, len(next.GetEntries())-1)
		for i, candidate := range next.GetEntries() {
			if i == found {
				continue
			}
			filtered = append(filtered, candidate.CloneVT())
		}
		next.Entries = filtered
		return next, true
	}
	if found >= 0 {
		if next.GetEntries()[found].EqualVT(entry) {
			return next, false
		}
		next.Entries[found] = entry.CloneVT()
		return next, true
	}
	next.Entries = append(next.GetEntries(), entry.CloneVT())
	return next, true
}

func cloneResponse(resp *api.GetMailboxResponse) *api.GetMailboxResponse {
	if resp == nil {
		return nil
	}
	return resp.CloneVT()
}

func cloneEntries(
	entries []*s4wave_provider_spacewave.MailboxEntryInfo,
) []*s4wave_provider_spacewave.MailboxEntryInfo {
	if len(entries) == 0 {
		return nil
	}
	next := make([]*s4wave_provider_spacewave.MailboxEntryInfo, 0, len(entries))
	for _, entry := range entries {
		next = append(next, cloneEntry(entry))
	}
	return next
}

func cloneEntry(
	entry *s4wave_provider_spacewave.MailboxEntryInfo,
) *s4wave_provider_spacewave.MailboxEntryInfo {
	if entry == nil {
		return nil
	}
	return &s4wave_provider_spacewave.MailboxEntryInfo{
		Id:        entry.GetId(),
		InviteId:  entry.GetInviteId(),
		PeerId:    entry.GetPeerId(),
		Status:    entry.GetStatus(),
		CreatedAt: entry.GetCreatedAt(),
		AccountId: entry.GetAccountId(),
		EntityId:  entry.GetEntityId(),
	}
}

func entriesEqual(
	aEntries []*s4wave_provider_spacewave.MailboxEntryInfo,
	bEntries []*s4wave_provider_spacewave.MailboxEntryInfo,
) bool {
	if len(aEntries) != len(bEntries) {
		return false
	}
	for i := range aEntries {
		aEntry := aEntries[i]
		bEntry := bEntries[i]
		if aEntry == nil || bEntry == nil {
			if aEntry != bEntry {
				return false
			}
			continue
		}
		if aEntry.GetId() != bEntry.GetId() ||
			aEntry.GetInviteId() != bEntry.GetInviteId() ||
			aEntry.GetPeerId() != bEntry.GetPeerId() ||
			aEntry.GetStatus() != bEntry.GetStatus() ||
			aEntry.GetCreatedAt() != bEntry.GetCreatedAt() ||
			aEntry.GetAccountId() != bEntry.GetAccountId() ||
			aEntry.GetEntityId() != bEntry.GetEntityId() {
			return false
		}
	}
	return true
}

func responseEqual(
	aResp *api.GetMailboxResponse,
	bResp *api.GetMailboxResponse,
) bool {
	if aResp == nil || bResp == nil {
		return aResp == bResp
	}
	return aResp.EqualVT(bResp)
}

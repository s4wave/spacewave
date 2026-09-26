package sobject

import (
	"context"
	"encoding/hex"
	"slices"
	"sync"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/net/crypto"
)

// TestLeaveSOParticipantsRebasesIndependentDeparture proves that one person's
// stale replica can leave after another participant advances the owner config.
func TestLeaveSOParticipantsRebasesIndependentDeparture(t *testing.T) {
	ctx := t.Context()
	peers := createMockPeers(t, 3)
	keys := make([]crypto.PrivKey, len(peers))
	participants := make([]*SOParticipantConfig, len(peers))
	for i, candidate := range peers {
		key, err := candidate.GetPrivKey(ctx)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
		role := SOParticipantRole_SOParticipantRole_WRITER
		if i == 0 {
			role = SOParticipantRole_SOParticipantRole_OWNER
		}
		participants[i] = &SOParticipantConfig{PeerId: candidate.GetPeerID().String(), Role: role}
	}
	initial := &SharedObjectConfig{Participants: participants}
	genesis, err := BuildSOConfigChange(initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := VerifyConfigChange(initial, genesis)
	if err != nil {
		t.Fatal(err)
	}
	host, state := newLeaveTestHost(t, checkpoint, genesis)

	first, err := BuildSOLeaveRequest(mockSharedObjectID, checkpoint.GetConfigChainHash(), keys[1])
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildSOLeaveRequest(mockSharedObjectID, checkpoint.GetConfigChainHash(), keys[2])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LeaveSOParticipants(ctx, host, keys[0], first); err != nil {
		t.Fatal(err)
	}
	response, err := LeaveSOParticipants(ctx, host, keys[0], second)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.GetChanges()) != 2 {
		t.Fatalf("rebased leave returned %d changes, want 2", len(response.GetChanges()))
	}
	current := (*state).GetConfig()
	if len(current.GetParticipants()) != 1 || current.GetParticipants()[0].GetPeerId() != peers[0].GetPeerID().String() {
		t.Fatalf("independent departures left audience %v", current.GetParticipants())
	}
	if err := VerifyConfigChainSuffix(checkpoint, current, response.GetChanges()); err != nil {
		t.Fatalf("rebased response does not prove departure: %v", err)
	}
}

// TestLeaveSOParticipantsRebasesAcrossAdmission proves that a replica which
// has not yet observed another participant's admission can still leave.
func TestLeaveSOParticipantsRebasesAcrossAdmission(t *testing.T) {
	ctx := t.Context()
	peers := createMockPeers(t, 3)
	keys := make([]crypto.PrivKey, len(peers))
	for i, candidate := range peers {
		key, err := candidate.GetPrivKey(ctx)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
	}
	initial := &SharedObjectConfig{Participants: []*SOParticipantConfig{
		{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: peers[1].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_WRITER},
	}}
	genesis, err := BuildSOConfigChange(initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := VerifyConfigChange(initial, genesis)
	if err != nil {
		t.Fatal(err)
	}
	host, state := newLeaveTestHost(t, checkpoint, genesis)
	request, err := BuildSOLeaveRequest(mockSharedObjectID, checkpoint.GetConfigChainHash(), keys[1])
	if err != nil {
		t.Fatal(err)
	}

	admitted := checkpoint.CloneVT()
	admitted.Participants = append(admitted.Participants, &SOParticipantConfig{
		PeerId: peers[2].GetPeerID().String(),
		Role:   SOParticipantRole_SOParticipantRole_WRITER,
	})
	admission, err := BuildSOConfigChange(checkpoint, admitted, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.ApplyConfigChange(ctx, admission, nil); err != nil {
		t.Fatal(err)
	}

	response, err := LeaveSOParticipants(ctx, host, keys[0], request)
	if err != nil {
		t.Fatal(err)
	}
	current := (*state).GetConfig()
	if err := VerifyConfigChainSuffix(checkpoint, current, response.GetChanges()); err != nil {
		t.Fatalf("rebased response does not prove departure: %v", err)
	}
	if slices.ContainsFunc(current.GetParticipants(), func(p *SOParticipantConfig) bool {
		return p.GetPeerId() == peers[1].GetPeerID().String()
	}) || len(current.GetParticipants()) != 2 {
		t.Fatalf("admission rebase left audience %v", current.GetParticipants())
	}
}

// TestLeaveSOParticipantsRejectsPriorAdmissionConsent keeps an old request from
// removing the same cryptographic identity after removal and readmission.
func TestLeaveSOParticipantsRejectsPriorAdmissionConsent(t *testing.T) {
	ctx := t.Context()
	peers := createMockPeers(t, 2)
	owner, err := peers[0].GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	departing, err := peers[1].GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	initial := &SharedObjectConfig{Participants: []*SOParticipantConfig{
		{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: peers[1].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_WRITER},
	}}
	genesis, err := BuildSOConfigChange(initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := VerifyConfigChange(initial, genesis)
	if err != nil {
		t.Fatal(err)
	}
	host, state := newLeaveTestHost(t, checkpoint, genesis)
	request, err := BuildSOLeaveRequest(mockSharedObjectID, checkpoint.GetConfigChainHash(), departing)
	if err != nil {
		t.Fatal(err)
	}

	removed := checkpoint.CloneVT()
	removed.Participants = removed.Participants[:1]
	removal, err := BuildSOConfigChange(checkpoint, removed, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.ApplyConfigChange(ctx, removal, nil); err != nil {
		t.Fatal(err)
	}
	readmitted := (*state).GetConfig().CloneVT()
	readmitted.Participants = append(readmitted.Participants, initial.GetParticipants()[1].CloneVT())
	admission, err := BuildSOConfigChange((*state).GetConfig(), readmitted, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.ApplyConfigChange(ctx, admission, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := LeaveSOParticipants(ctx, host, owner, request); err == nil {
		t.Fatal("prior-admission consent removed a rejoined participant")
	}
	if !slices.ContainsFunc((*state).GetConfig().GetParticipants(), func(p *SOParticipantConfig) bool {
		return p.GetPeerId() == peers[1].GetPeerID().String()
	}) {
		t.Fatal("rejected leave changed current participation")
	}
}

// newLeaveTestHost retains signed config history behind the same lock as state.
func newLeaveTestHost(t *testing.T, config *SharedObjectConfig, retained ...*SOConfigChange) (*SOHost, **SOState) {
	t.Helper()
	state := &SOState{Config: config.CloneVT()}
	statePtr := &state
	ctr := ccontainer.NewCContainer[*SOState](state)
	entries := make(map[string]*SOConfigChange)
	var mu sync.Mutex
	retain := func(changes []*SOConfigChange) error {
		for _, change := range changes {
			hash, err := HashSOConfigChange(change)
			if err != nil {
				return err
			}
			entries[hex.EncodeToString(hash)] = change.CloneVT()
		}
		return nil
	}
	if err := retain(retained); err != nil {
		t.Fatal(err)
	}
	entry := func(_ context.Context, head []byte) (*SOConfigChange, error) {
		return entries[hex.EncodeToString(head)], nil
	}
	history := func(ctx context.Context, _ string, base, target []byte) ([]*SOConfigChange, error) {
		mu.Lock()
		defer mu.Unlock()
		return ReadConfigSuffix(ctx, base, target, entry)
	}
	return NewSOHost(t.Context(), func(_ context.Context, _ string, _ func()) (ccontainer.Watchable[*SOState], func(), error) {
		return ctr, func() {}, nil
	}, func(_ context.Context, _ string) (SOStateLock, error) {
		mu.Lock()
		return NewSOStateLock(*statePtr, func(_ context.Context, next *SOState, changes ...*SOConfigChange) error {
			if err := retain(changes); err != nil {
				return err
			}
			*statePtr = next
			ctr.SetValue(next)
			return nil
		}, mu.Unlock), nil
	}, mockSharedObjectID, &SOHostSyncFuncs{History: history, Entry: func(ctx context.Context, _ string, head []byte) (*SOConfigChange, error) {
		mu.Lock()
		defer mu.Unlock()
		return entry(ctx, head)
	}}), statePtr
}

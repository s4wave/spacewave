package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/bstore"
	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"github.com/sirupsen/logrus"
)

func TestBuildSessionPresentationReconcileStateLocked(t *testing.T) {
	acc := &ProviderAccount{}
	acc.state.info = &api.AccountStateResponse{
		AccountSobjectBindings: []*api.AccountSObjectBinding{
			{
				Purpose: account_settings.BindingPurpose,
				SoId:    "so-settings",
				State:   api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_READY,
			},
		},
	}
	acc.state.sessionsValid = true
	acc.state.sessions = []*api.AccountSessionInfo{
		{PeerId: "peer-b"},
		{PeerId: "peer-a"},
		{PeerId: ""},
	}

	got := acc.buildSessionPresentationReconcileStateLocked()
	if got == nil {
		t.Fatal("expected reconcile state")
	}
	if got.accountSettingsSOID != "so-settings" {
		t.Fatalf("expected so id %q, got %q", "so-settings", got.accountSettingsSOID)
	}
	if !slices.Equal(got.liveSessionPeerIDs, []string{"peer-a", "peer-b"}) {
		t.Fatalf("unexpected live peer IDs: %v", got.liveSessionPeerIDs)
	}
}

func TestBuildSessionPresentationReconcileStateLockedRequiresReadyBinding(t *testing.T) {
	acc := &ProviderAccount{}
	acc.state.info = &api.AccountStateResponse{
		AccountSobjectBindings: []*api.AccountSObjectBinding{
			{
				Purpose: account_settings.BindingPurpose,
				SoId:    "so-settings",
				State:   api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_RESERVED,
			},
		},
	}
	acc.state.sessionsValid = true
	acc.state.sessions = []*api.AccountSessionInfo{{PeerId: "peer-a"}}

	if got := acc.buildSessionPresentationReconcileStateLocked(); got != nil {
		t.Fatalf("expected nil reconcile state for non-ready binding, got %+v", got)
	}
}

func TestBuildSessionPresentationReconcileStateLockedSkipsReadOnlyLifecycle(t *testing.T) {
	acc := &ProviderAccount{}
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_CANCELED_GRACE_READONLY,
		AccountSobjectBindings: []*api.AccountSObjectBinding{
			{
				Purpose: account_settings.BindingPurpose,
				SoId:    "so-settings",
				State:   api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_READY,
			},
		},
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
	acc.state.sessionsValid = true
	acc.state.sessions = []*api.AccountSessionInfo{{PeerId: "peer-a"}}

	if got := acc.buildSessionPresentationReconcileStateLocked(); got != nil {
		t.Fatalf("expected nil reconcile state for read-only lifecycle, got %+v", got)
	}
}

func TestApplyFetchedAccountStateUpdatesSessionPresentationReconcileState(t *testing.T) {
	// Build an account with a reconcile routine container.
	acc := &ProviderAccount{}
	acc.sessionPresentationReconcile = routine.NewStateRoutineContainer(
		equalSessionPresentationReconcileState,
	)

	// Apply a fetched state with the settings binding and one live session.
	acc.applyFetchedAccountState(2, &api.AccountStateResponse{
		Epoch: 2,
		AccountSobjectBindings: []*api.AccountSObjectBinding{
			{
				Purpose: account_settings.BindingPurpose,
				SoId:    "so-settings",
				State:   api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_READY,
			},
		},
		Sessions: []*api.AccountSessionInfo{
			{PeerId: "peer-live"},
		},
	})

	// The reconcile state names the settings Space and the live session.
	got := acc.sessionPresentationReconcile.GetState()
	if got == nil {
		t.Fatal("expected reconcile state to be stored")
	}
	if got.accountSettingsSOID != "so-settings" {
		t.Fatalf("expected so id %q, got %q", "so-settings", got.accountSettingsSOID)
	}
	if !slices.Equal(got.liveSessionPeerIDs, []string{"peer-live"}) {
		t.Fatalf("unexpected live peer IDs: %v", got.liveSessionPeerIDs)
	}
}

func TestBuildOrphanedSessionPresentationPeerIDs(t *testing.T) {
	got := buildOrphanedSessionPresentationPeerIDs(
		[]string{"peer-live", "peer-orphan", "peer-other"},
		[]string{"peer-live"},
	)
	if !slices.Equal(got, []string{"peer-orphan", "peer-other"}) {
		t.Fatalf("unexpected orphaned peer IDs: %v", got)
	}
}

func TestReconcileSessionPresentationStateRemovesOrphans(t *testing.T) {
	settings := &account_settings.AccountSettings{
		SessionPresentations: []*account_settings.SessionPresentation{
			{PeerId: "peer-live"},
			{PeerId: "peer-orphan"},
		},
	}
	so := newTestSessionPresentationSharedObject(t, settings)
	acc := &ProviderAccount{}

	err := acc.reconcileSessionPresentationState(context.Background(), so, &sessionPresentationReconcileState{
		accountSettingsSOID: "so-settings",
		liveSessionPeerIDs:  []string{"peer-live"},
	})
	if err != nil {
		t.Fatalf("reconcile session presentation state: %v", err)
	}

	if len(so.removedPeerIDs) != 1 || so.removedPeerIDs[0] != "peer-orphan" {
		t.Fatalf("expected orphaned peer removal, got %v", so.removedPeerIDs)
	}
}

func TestRunSessionPresentationReconcileReadOnlySkipsCloudCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected account settings reconcile request: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_CANCELED_GRACE_READONLY,
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY

	err := acc.runSessionPresentationReconcile(context.Background(), &sessionPresentationReconcileState{
		accountSettingsSOID: "so-settings",
		liveSessionPeerIDs:  []string{"peer-live"},
	})
	if err != nil {
		t.Fatalf("run session presentation reconcile: %v", err)
	}
}

func TestUpsertSessionPresentationReadOnlySkipsCloudCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected account settings upsert request: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_CANCELED_GRACE_READONLY,
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY

	err := acc.UpsertSessionPresentation(context.Background(), "peer-live", &api.ObservedSessionMetadata{
		Label: "Desktop",
	})
	if err != nil {
		t.Fatalf("upsert session presentation: %v", err)
	}
}

// testSessionPresentationSharedObject is an account settings Shared Object
// over a real genesis state, owned by the local peer, that records each
// session presentation removal it is asked to write.
type testSessionPresentationSharedObject struct {
	t              *testing.T
	priv           crypto.PrivKey
	peerID         peer.ID
	state          *sobject.SOState
	snaps          *ccontainer.CContainer[sobject.SharedObjectStateSnapshot]
	removedPeerIDs []string
}

// newTestSessionPresentationSharedObject returns a Shared Object whose genesis
// checkpoint holds settings.
func newTestSessionPresentationSharedObject(
	t *testing.T,
	settings *account_settings.AccountSettings,
) *testSessionPresentationSharedObject {
	// Create an owner.
	t.Helper()
	owner, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := owner.GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	// Encode the settings into a genesis state the owner holds.
	data, err := settings.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	state, _, err := sobject.BuildGenesisSOState(logrus.NewEntry(logrus.New()), testStepFactorySet(), "so-settings", priv, data)
	if err != nil {
		t.Fatal(err)
	}
	s := &testSessionPresentationSharedObject{t: t, priv: priv, peerID: owner.GetPeerID(), state: state}
	s.snaps = ccontainer.NewCContainer(s.snapshot())
	return s
}

func (s *testSessionPresentationSharedObject) GetBus() bus.Bus {
	panic("unexpected GetBus call")
}

func (s *testSessionPresentationSharedObject) GetPeerID() peer.ID {
	return s.peerID
}

func (s *testSessionPresentationSharedObject) GetSharedObjectID() string {
	return "so-settings"
}

func (s *testSessionPresentationSharedObject) GetBlockStore() bstore.BlockStore {
	panic("unexpected GetBlockStore call")
}

func (s *testSessionPresentationSharedObject) AccessLocalStateStore(context.Context, string, func()) (kvtx.Store, func(), error) {
	panic("unexpected AccessLocalStateStore call")
}

func (s *testSessionPresentationSharedObject) GetSharedObjectState(context.Context) (sobject.SharedObjectStateSnapshot, error) {
	return s.snaps.GetValue(), nil
}

func (s *testSessionPresentationSharedObject) AccessSharedObjectState(
	context.Context,
	func(),
) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return s.snaps, func() {}, nil
}

// QueueOperation records a session presentation removal, then signs the
// operation encrypted with the current key and publishes the state holding it.
func (s *testSessionPresentationSharedObject) QueueOperation(
	ctx context.Context,
	op []byte,
) (string, error) {
	// Record the removal.
	msg := &account_settings.AccountSettingsOp{}
	if err := msg.UnmarshalVT(op); err != nil {
		return "", err
	}
	rm := msg.GetRemoveSessionPresentation()
	if rm == nil {
		s.t.Fatalf("expected remove session presentation op, got %T", msg.GetOp())
	}
	s.removedPeerIDs = append(s.removedPeerIDs, rm.GetPeerId())

	// Encrypt the operation and link it after the owner's last one.
	xfrm, err := s.snaps.GetValue().GetTransformer(ctx)
	if err != nil {
		return "", err
	}
	set, err := s.state.OperationSet(s.GetSharedObjectID())
	if err != nil {
		return "", err
	}
	link := s.state.NextOperationLink(set, s.peerID.String())
	data, err := xfrm.EncodeBlock(op)
	if err != nil {
		return "", err
	}

	// Sign the operation, add it, and publish the state.
	localID := sobject.NewSOOperationLocalID()
	signed, err := sobject.BuildSOOperation(s.GetSharedObjectID(), s.priv, data, link, localID)
	if err != nil {
		return "", err
	}
	if _, err := s.state.AddOperation(s.GetSharedObjectID(), signed); err != nil {
		return "", err
	}
	s.snaps.SetValue(s.snapshot())
	return localID, nil
}

// snapshot returns the owner's handle over a copy of the current state.
func (s *testSessionPresentationSharedObject) snapshot() sobject.SharedObjectStateSnapshot {
	return sobject.NewSOStateParticipantHandle(
		logrus.NewEntry(logrus.New()),
		testStepFactorySet(),
		s.GetSharedObjectID(),
		s.state.CloneVT(),
		s.priv,
		s.peerID,
	)
}

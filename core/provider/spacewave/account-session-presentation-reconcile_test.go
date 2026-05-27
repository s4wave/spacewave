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
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
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
	acc := &ProviderAccount{}
	acc.sessionPresentationReconcile = routine.NewStateRoutineContainer(
		equalSessionPresentationReconcileState,
	)

	acc.applyFetchedAccountState(2, &api.AccountStateResponse{
		Epoch: 2,
		AccountSobjectBindings: []*api.AccountSObjectBinding{
			{
				Purpose: account_settings.BindingPurpose,
				SoId:    "so-settings",
				State:   api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_READY,
			},
		},
	}, nil, []*api.AccountSessionInfo{
		{PeerId: "peer-live"},
	})

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

type testSessionPresentationSharedObject struct {
	t              *testing.T
	settings       *account_settings.AccountSettings
	removedPeerIDs []string
}

func newTestSessionPresentationSharedObject(
	t *testing.T,
	settings *account_settings.AccountSettings,
) *testSessionPresentationSharedObject {
	t.Helper()
	return &testSessionPresentationSharedObject{
		t:        t,
		settings: settings.CloneVT(),
	}
}

func (s *testSessionPresentationSharedObject) GetBus() bus.Bus {
	panic("unexpected GetBus call")
}

func (s *testSessionPresentationSharedObject) GetPeerID() peer.ID {
	panic("unexpected GetPeerID call")
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
	return s.snapshot(), nil
}

func (s *testSessionPresentationSharedObject) AccessSharedObjectState(
	context.Context,
	func(),
) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	ctr := ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](s.snapshot())
	return ctr, func() {}, nil
}

func (s *testSessionPresentationSharedObject) QueueOperation(
	_ context.Context,
	op []byte,
) (string, error) {
	msg := &account_settings.AccountSettingsOp{}
	if err := msg.UnmarshalVT(op); err != nil {
		return "", err
	}
	rm := msg.GetRemoveSessionPresentation()
	if rm == nil {
		s.t.Fatalf("expected remove session presentation op, got %T", msg.GetOp())
	}
	s.removedPeerIDs = append(s.removedPeerIDs, rm.GetPeerId())
	return rm.GetPeerId(), nil
}

func (s *testSessionPresentationSharedObject) WaitOperation(
	context.Context,
	string,
) (uint64, bool, error) {
	return 1, false, nil
}

func (s *testSessionPresentationSharedObject) ClearOperationResult(
	context.Context,
	string,
) error {
	return nil
}

func (s *testSessionPresentationSharedObject) ProcessOperations(
	context.Context,
	bool,
	sobject.ProcessOpsFunc,
) error {
	panic("unexpected ProcessOperations call")
}

func (s *testSessionPresentationSharedObject) snapshot() sobject.SharedObjectStateSnapshot {
	data, err := s.settings.MarshalVT()
	if err != nil {
		s.t.Fatalf("marshal settings: %v", err)
	}
	return &testSessionPresentationSnapshot{
		rootInner: &sobject.SORootInner{
			StateData: data,
		},
	}
}

type testSessionPresentationSnapshot struct {
	rootInner *sobject.SORootInner
}

func (s *testSessionPresentationSnapshot) GetParticipantConfig(context.Context) (*sobject.SOParticipantConfig, error) {
	panic("unexpected GetParticipantConfig call")
}

func (s *testSessionPresentationSnapshot) GetTransformer(context.Context) (*block_transform.Transformer, error) {
	panic("unexpected GetTransformer call")
}

func (s *testSessionPresentationSnapshot) GetTransformInfo(context.Context) (*sobject.TransformInfo, error) {
	panic("unexpected GetTransformInfo call")
}

func (s *testSessionPresentationSnapshot) GetOpQueue(context.Context) ([]*sobject.SOOperation, []*sobject.QueuedSOOperation, error) {
	panic("unexpected GetOpQueue call")
}

func (s *testSessionPresentationSnapshot) GetRootInner(context.Context) (*sobject.SORootInner, error) {
	return s.rootInner, nil
}

func (s *testSessionPresentationSnapshot) ProcessOperations(
	context.Context,
	[]*sobject.SOOperation,
	sobject.SnapshotProcessOpsFunc,
) (*sobject.SORoot, []*sobject.SOOperationRejection, []*sobject.SOOperation, error) {
	panic("unexpected ProcessOperations call")
}

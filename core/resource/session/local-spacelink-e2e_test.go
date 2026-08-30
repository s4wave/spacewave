package resource_session_test

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	resource_provider "github.com/s4wave/spacewave/core/resource/provider"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	core_session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	transport_dialer "github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_inproc "github.com/s4wave/spacewave/net/transport/inproc"
	s4wave_provider_local "github.com/s4wave/spacewave/sdk/provider/local"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

const localEnrollmentTestTimeout = 2 * time.Minute

// buildDeviceTicket builds a signed SpaceLink DEVICE ticket for the test.
func buildDeviceTicket(t *testing.T, priv crypto.PrivKey, agentPeerID peer.ID) (ticketBytes, nonce []byte) {
	t.Helper()

	nonce = make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	payload := &s4wave_provider_spacewave.SpaceLinkAuthRequest{
		Version:        1,
		SessionType:    core_session.SessionType_SESSION_TYPE_DEVICE,
		AgentPeerId:    []byte(agentPeerID),
		Label:          "e2e device",
		RequestedRole:  sobject.SOParticipantRole_SOParticipantRole_WRITER,
		Nonce:          nonce,
		ExpiresAt:      time.Now().Add(15 * time.Minute).Unix(),
		CompletionMode: s4wave_provider_spacewave.SpaceLinkCompletionMode_SpaceLinkCompletionMode_CLI,
	}
	payloadBytes, err := payload.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	sig, err := priv.Sign(payloadBytes)
	if err != nil {
		t.Fatal(err)
	}
	ticketBytes, err = (&s4wave_provider_spacewave.SpaceLinkAuthTicket{
		Payload:        payloadBytes,
		AgentSignature: sig,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return ticketBytes, nonce
}

// connectSessionTransportsForTest connects two session transports via inproc
// and waits for one established link. Dual-dial from both sides is unstable.
func connectSessionTransportsForTest(ctx context.Context, t *testing.T, stA, stB *transport.SessionTransport) {
	t.Helper()

	peerIDA := stA.GetPeerID()
	peerIDB := stB.GetPeerID()
	childBusA := stA.GetChildBus()
	childBusB := stB.GetChildBus()

	le := logrus.NewEntry(logrus.New())

	inprocCtrlA := transport_inproc.BuildInprocController(le, childBusA, "", &transport_inproc.Config{
		Dialers: map[string]*transport_dialer.DialerOpts{
			peerIDB.String(): {Address: transport_inproc.NewAddr(peerIDB).String()},
		},
	})
	inprocCtrlB := transport_inproc.BuildInprocController(le, childBusB, "", &transport_inproc.Config{
		Dialers: map[string]*transport_dialer.DialerOpts{
			peerIDA.String(): {Address: transport_inproc.NewAddr(peerIDA).String()},
		},
	})

	if _, err := childBusA.AddController(ctx, inprocCtrlA, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := childBusB.AddController(ctx, inprocCtrlB, nil); err != nil {
		t.Fatal(err)
	}

	tptA, err := inprocCtrlA.GetTransport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tptB, err := inprocCtrlB.GetTransport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ipA := tptA.(*transport_inproc.Inproc)
	ipB := tptB.(*transport_inproc.Inproc)
	ipA.ConnectToInproc(ctx, ipB)
	ipB.ConnectToInproc(ctx, ipA)

	_, rel, err := link.EstablishLinkWithPeerEx(ctx, childBusA, peerIDA, peerIDB, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rel)
}

// TestLocalSpaceLinkDeviceEnrollmentEndToEnd covers the full local Device
// enrollment slice in memory: ticket approval by the OWNER, non-owner
// rejection before mutation, nonce replay rejection, one-use targeted invite
// completion from the Device's own key, and restart remount identity.
func TestLocalSpaceLinkDeviceEnrollmentEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in-memory enrollment e2e in short mode")
	}

	ctx, cancel := context.WithTimeout(t.Context(), localEnrollmentTestTimeout)
	defer cancel()

	ownerEnv := setupTestEnv(ctx, t)
	deviceEnv := setupTestEnv(ctx, t)

	// Owner account with one Space.
	ownerSessRef, _ := ownerEnv.createSession(ctx, t)
	ownerAcc := ownerEnv.accessAccount(ctx, t, ownerSessRef)
	ownerEnv.createSpaceOnAccount(ctx, t, ownerAcc, "EnrollSpace")
	spaceID := "enrollspace-id"

	// Keep the owner Space mounted so the invite handshake reaches the
	// SOHost for the duration of the enrollment.
	ownerSORef := sobject.NewSharedObjectRef(
		"local",
		ownerSessRef.GetProviderResourceRef().GetProviderAccountId(),
		spaceID,
		provider_local.SobjectBlockStoreID(spaceID),
	)
	ownerSOIface, relOwnerSO, err := ownerAcc.MountSharedObject(ctx, ownerSORef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relOwnerSO()
	ownerMountedSO, ok := ownerSOIface.(*provider_local.SharedObject)
	if !ok {
		t.Fatal("unexpected owner shared object type")
	}

	ownerSess, relOwnerSess, err := ownerAcc.MountSession(ctx, ownerSessRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relOwnerSess()

	// A second, non-owner local session cannot approve the ticket. The
	// failed attempt must not consume the ticket nonce.
	otherSessRef, _ := ownerEnv.createSession(ctx, t)
	otherAcc := ownerEnv.accessAccount(ctx, t, otherSessRef)
	otherSess, relOtherSess, err := otherAcc.MountSession(ctx, otherSessRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relOtherSess()

	devicePriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	devicePeerID, err := peer.IDFromPrivateKey(devicePriv)
	if err != nil {
		t.Fatal(err)
	}
	devicePEM, err := keypem.MarshalPrivKeyPem(devicePriv)
	if err != nil {
		t.Fatal(err)
	}
	otherPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherPEM, err := keypem.MarshalPrivKeyPem(otherPriv)
	if err != nil {
		t.Fatal(err)
	}
	ticketBytes, nonce := buildDeviceTicket(t, devicePriv, devicePeerID)

	// Non-owner approval is rejected before any mutation.
	nonOwnerRes := resource_session.NewLocalSessionResource(ownerEnv.tb.Bus, otherSess)
	_, err = nonOwnerRes.ApproveSpaceLink(ctx, &s4wave_session.ApproveLocalSpaceLinkRequest{
		Ticket:     ticketBytes,
		ResourceId: spaceID,
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected non-owner rejection, got %v", err)
	}

	// The OWNER approves the ticket.
	ownerRes := resource_session.NewLocalSessionResource(ownerEnv.tb.Bus, ownerSess)
	approvalCtx, cancelApproval := context.WithCancel(ctx)
	resp, err := ownerRes.ApproveSpaceLink(approvalCtx, &s4wave_session.ApproveLocalSpaceLinkRequest{
		Ticket:     ticketBytes,
		ResourceId: spaceID,
	})
	cancelApproval()
	if err != nil {
		t.Fatal(err)
	}
	completion := resp.GetCompletion()
	if completion == nil {
		t.Fatal("approval returned no completion")
	}
	if completion.GetProviderId() != "local" {
		t.Fatalf("unexpected provider id %q", completion.GetProviderId())
	}
	if completion.GetResourceId() != spaceID {
		t.Fatalf("unexpected resource id %q", completion.GetResourceId())
	}
	if string(completion.GetSessionPeerId()) != string(devicePeerID) {
		t.Fatal("completion peer does not match device identity")
	}
	if string(completion.GetNonce()) != string(nonce) {
		t.Fatal("completion nonce does not match ticket nonce")
	}
	invite := completion.GetInvite()
	if invite == nil {
		t.Fatal("completion carries no invite")
	}
	if invite.GetTargetPeerId() != devicePeerID.String() {
		t.Fatal("invite is not targeted at the device peer")
	}
	if invite.GetMaxUses() != 1 {
		t.Fatalf("invite is not one-use: %d", invite.GetMaxUses())
	}
	if invite.GetRole() != sobject.SOParticipantRole_SOParticipantRole_WRITER {
		t.Fatal("invite role does not match requested role")
	}

	// Replaying the same ticket is rejected.
	_, err = ownerRes.ApproveSpaceLink(ctx, &s4wave_session.ApproveLocalSpaceLinkRequest{
		Ticket:     ticketBytes,
		ResourceId: spaceID,
	})
	if err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("expected nonce replay rejection, got %v", err)
	}

	// The enrollment RPC rejects a key that does not match the expected
	// session peer id before creating any local state.
	le := logrus.NewEntry(logrus.New())
	deviceProvRes := resource_provider.NewLocalProviderResource(
		resource_provider.NewProviderResource(le, deviceEnv.tb.Bus, deviceEnv.prov),
		le,
		deviceEnv.tb.Bus,
		deviceEnv.prov,
	)
	_, err = deviceProvRes.CompleteSpaceLinkEnrollment(ctx, &s4wave_provider_local.CompleteSpaceLinkEnrollmentRequest{
		SessionPemPrivateKey: otherPEM,
		SessionPeerId:        devicePeerID.String(),
		Invite:               invite,
	})
	if err == nil || !strings.Contains(err.Error(), "does not match expected session peer id") {
		t.Fatalf("expected peer id mismatch rejection, got %v", err)
	}

	// The Device daemon creates its own local session from its durable key
	// and joins the Space through the one-use targeted invite.
	deviceSessRef, err := deviceEnv.prov.CreateLocalAccountAndSessionWithKey(ctx, "", devicePEM)
	if err != nil {
		t.Fatal(err)
	}
	if deviceSessRef.GetProviderResourceRef().GetProviderId() != "local" {
		t.Fatal("device session is not a local session")
	}
	deviceAccIface, relDeviceAcc, err := deviceEnv.prov.AccessProviderAccount(
		ctx,
		deviceSessRef.GetProviderResourceRef().GetProviderAccountId(),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer relDeviceAcc()
	deviceAcc := deviceAccIface.(*provider_local.ProviderAccount)

	deviceSess, relDeviceSess, err := deviceAcc.MountSession(ctx, deviceSessRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	if deviceSess.GetPeerId().String() != devicePeerID.String() {
		t.Fatal("device session did not reopen with the supplied key identity")
	}

	// Approval starts the owner's invite service on its mounted session
	// transport. Connect the Device transport, then exercise the same invite
	// RPC that the cross-host CLI uses.
	ownerTransport := ownerAcc.GetSessionTransport()
	if ownerTransport == nil {
		t.Fatal("approval did not retain the owner session transport")
	}
	defer ownerAcc.StopSessionTransport()
	defer ownerAcc.StopP2PSync()
	if err := deviceAcc.EnsureSessionTransport(ctx, devicePriv, ""); err != nil {
		t.Fatal(err)
	}
	defer deviceAcc.StopSessionTransport()
	connectSessionTransportsForTest(ctx, t, deviceAcc.GetSessionTransport(), ownerTransport)

	joinCtx, cancelJoin := context.WithCancel(ctx)
	joinResult, err := deviceAcc.JoinViaInvite(joinCtx, devicePriv, invite, "")
	cancelJoin()
	if err != nil {
		t.Fatal(err)
	}
	if joinResult == nil || joinResult.Grant == nil {
		t.Fatal("invite join returned no grant")
	}
	joinRelease := time.NewTimer(250 * time.Millisecond)
	defer joinRelease.Stop()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-joinRelease.C:
	}
	if err := deviceAcc.RetainP2PPeer(ctx, ownerTransport.GetPeerID()); err != nil {
		t.Fatalf("enrollment request released P2P sync: %v", err)
	}
	if joinResult.SharedObjectID != spaceID {
		t.Fatalf("unexpected joined shared object %q", joinResult.SharedObjectID)
	}
	if joinResult.Grant.GetPeerId() != devicePeerID.String() {
		t.Fatal("grant does not target the device peer")
	}

	// The grant and Space membership persist for the device account.
	deviceAccountID := deviceSessRef.GetProviderResourceRef().GetProviderAccountId()
	deviceSORef := sobject.NewSharedObjectRef("local", deviceAccountID, spaceID, provider_local.SobjectBlockStoreID(spaceID))
	deviceSO, relDeviceSO, err := deviceAcc.MountSharedObject(ctx, deviceSORef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relDeviceSO()
	localDeviceSO, ok := deviceSO.(*provider_local.SharedObject)
	if !ok {
		t.Fatal("unexpected shared object type")
	}
	deviceSOState, err := localDeviceSO.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foundGrant := false
	for _, grant := range deviceSOState.GetRootGrants() {
		if grant.GetPeerId() == devicePeerID.String() {
			foundGrant = true
		}
	}
	if !foundGrant {
		t.Fatal("device SO state is missing the device grant")
	}
	foundEntry := false
	for _, soEntry := range deviceAcc.GetSOListCtr().GetValue().GetSharedObjects() {
		if soEntry.GetRef().GetProviderResourceRef().GetId() == spaceID && soEntry.GetSource() == "shared" {
			foundEntry = true
		}
	}
	if !foundEntry {
		t.Fatal("device SO list is missing the joined Space")
	}

	ownerState, err := ownerMountedSO.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foundParticipant := false
	for _, participant := range ownerState.GetConfig().GetParticipants() {
		if participant.GetPeerId() == devicePeerID.String() &&
			participant.GetRole() == sobject.SOParticipantRole_SOParticipantRole_WRITER {
			foundParticipant = true
		}
	}
	if !foundParticipant {
		t.Fatal("owner space is missing the enrolled Device participant")
	}

	// The joined Device copy contains a local storage signer marked OWNER,
	// but it did not originate the Space and cannot enroll another Device.
	childPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	childPeerID, err := peer.IDFromPrivateKey(childPriv)
	if err != nil {
		t.Fatal(err)
	}
	childTicket, _ := buildDeviceTicket(t, childPriv, childPeerID)
	deviceRes := resource_session.NewLocalSessionResource(deviceEnv.tb.Bus, deviceSess)
	_, err = deviceRes.ApproveSpaceLink(ctx, &s4wave_session.ApproveLocalSpaceLinkRequest{
		Ticket:     childTicket,
		ResourceId: spaceID,
	})
	if err == nil || !strings.Contains(err.Error(), "originating local account") {
		t.Fatalf("expected joined Device approval rejection, got %v", err)
	}

	// Restart remount: release the device session, then re-mount it. The
	// session tracker reloads its stored key and identity, and the grant
	// remains on the persisted Space.
	relDeviceSO()
	relDeviceSess()
	restartedSess, relRestartedSess, err := deviceAcc.MountSession(ctx, deviceSessRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relRestartedSess()
	if restartedSess.GetPeerId().String() != devicePeerID.String() {
		t.Fatal("remounted session lost its device identity")
	}
	restartedSO, relRestartedSO, err := deviceAcc.MountSharedObject(ctx, deviceSORef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relRestartedSO()
	localRestartedSO, ok := restartedSO.(*provider_local.SharedObject)
	if !ok {
		t.Fatal("unexpected shared object type after remount")
	}
	restartedState, err := localRestartedSO.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foundGrant = false
	for _, grant := range restartedState.GetRootGrants() {
		if grant.GetPeerId() == devicePeerID.String() {
			foundGrant = true
		}
	}
	if !foundGrant {
		t.Fatal("remounted device SO state is missing the device grant")
	}
}

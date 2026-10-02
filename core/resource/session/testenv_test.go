package resource_session_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// testEnv is a testbed with a local provider and a Session controller.
type testEnv struct {
	tb       *testbed.Testbed
	prov     *provider_local.Provider
	sessCtrl session.SessionController
}

// setupTestEnv builds a testEnv whose controllers stop with the test.
func setupTestEnv(ctx context.Context, t *testing.T) *testEnv {
	// Start the testbed.
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Register the controller factories.
	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(space_sobject.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))

	// Load the Session controller.
	_, sessCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{
		VolumeId: tb.EngineVolumeID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sessCtrlRef.Release)

	// Load the local provider on the testbed volume.
	peerID := tb.Volume.GetPeerID()
	providerID := "local"
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provCtrlRef.Release)

	// Load the Space body controller.
	_, spaceCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&space_sobject.Config{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(spaceCtrlRef.Release)

	// Look up the provider and Session controller.
	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, providerID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	sessCtrl, sessCtrlLookupRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sessCtrlLookupRef.Release)
	return &testEnv{
		tb:       tb,
		prov:     prov.(*provider_local.Provider),
		sessCtrl: sessCtrl,
	}
}

func (e *testEnv) createSession(ctx context.Context, t *testing.T) (*session.SessionRef, uint32) {
	// Report failures at the caller.
	t.Helper()

	// Create a local account and session.
	sessRef, err := e.prov.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	// Register the session and return its list index.
	entry, err := e.sessCtrl.RegisterSession(ctx, sessRef, &session.SessionMetadata{
		ProviderDisplayName: "Local",
		ProviderId:          "local",
		ProviderAccountId:   sessRef.GetProviderResourceRef().GetProviderAccountId(),
	})
	if err != nil {
		t.Fatal(err)
	}

	return sessRef, entry.GetSessionIndex()
}

func (e *testEnv) accessAccount(ctx context.Context, t *testing.T, sessRef *session.SessionRef) *provider_local.ProviderAccount {
	// Report failures at the caller.
	t.Helper()

	// Open the local provider account for the session.
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	accIface, accRel, err := e.prov.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(accRel)
	return accIface.(*provider_local.ProviderAccount)
}

func (e *testEnv) createSpaceOnAccount(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, spaceName string) {
	// Report failures at the caller.
	t.Helper()

	// Create a Space SharedObject on the account.
	meta, err := space.NewSharedObjectMeta(spaceName)
	if err != nil {
		t.Fatal(err)
	}
	soID := strings.ToLower(spaceName) + "-id"
	if _, err := acc.CreateSharedObject(ctx, soID, meta, "", ""); err != nil {
		t.Fatal(err)
	}
}

func (e *testEnv) buildSessionResource(ctx context.Context, t *testing.T, sessRef *session.SessionRef) *resource_session.SessionResource {
	// Report failures at the caller.
	t.Helper()

	// Mount the session on the test bus.
	sess, sessRelRef, err := session.ExMountSession(ctx, e.tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sessRelRef.Release)

	// Return a session resource for that mount.
	le := logrus.NewEntry(logrus.StandardLogger())
	return resource_session.NewSessionResource(le, e.tb.Bus, sess)
}

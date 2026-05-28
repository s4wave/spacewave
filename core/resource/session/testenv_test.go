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

type testEnv struct {
	tb       *testbed.Testbed
	prov     *provider_local.Provider
	sessCtrl session.SessionController
}

func setupTestEnv(ctx context.Context, t *testing.T) *testEnv {
	t.Helper()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(space_sobject.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))

	peerID := tb.Volume.GetPeerID()
	providerID := "local"

	_, sessCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{
		VolumeId: tb.EngineVolumeID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sessCtrlRef.Release)

	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provCtrlRef.Release)

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
	t.Helper()

	sessRef, err := e.prov.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

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
	t.Helper()

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	accIface, accRel, err := e.prov.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(accRel)
	return accIface.(*provider_local.ProviderAccount)
}

func (e *testEnv) createSpaceOnAccount(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, spaceName string) {
	t.Helper()

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
	t.Helper()

	sess, sessRelRef, err := session.ExMountSession(ctx, e.tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sessRelRef.Release)

	le := logrus.NewEntry(logrus.StandardLogger())
	return resource_session.NewSessionResource(le, e.tb.Bus, sess)
}

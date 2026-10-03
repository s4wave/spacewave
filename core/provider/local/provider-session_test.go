package provider_local_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/ccontainer"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/testbed"
)

// setupProviderAndSession creates a testbed with a local provider, account, and session.
// Optional trailing arguments set the standalone signaling URL and signing namespace.
func setupProviderAndSession(ctx context.Context, t *testing.T, signalingURL ...string) (
	*testbed.Testbed,
	*session.SessionRef,
	*provider_local.ProviderAccount,
	*provider_local.Session,
	func(),
) {
	t.Helper()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	providerID := "local"
	peerID := tb.Volume.GetPeerID()
	provCfg := &provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
		StorageId:  tb.StorageID,
	}
	if len(signalingURL) != 0 {
		provCfg.SignalingUrl = signalingURL[0]
	}
	if len(signalingURL) > 1 {
		provCfg.SignalingEnvPrefix = signalingURL[1]
	}
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(provCfg), nil)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}

	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, providerID, false, nil)
	if err != nil {
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	localProv := prov.(*provider_local.Provider)
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	accIface, accRel, err := localProv.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	acc := accIface.(*provider_local.ProviderAccount)

	sess, sessRelease, err := acc.MountSession(ctx, sessRef, nil)
	if err != nil {
		accRel()
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	localSess := sess.(*provider_local.Session)

	release := func() {
		sessRelease()
		accRel()
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
	}
	return tb, sessRef, acc, localSess, release
}

// mountAccountSettingsSO mounts the account settings SO via the bus.
func mountAccountSettingsSO(ctx context.Context, t *testing.T, b bus.Bus, accountID string) (sobject.SharedObject, func()) {
	t.Helper()

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()

	accIface, accRel, err := prov.(*provider_local.Provider).AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer accRel()

	ref, err := accIface.(*provider_local.ProviderAccount).GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}

	so, mountRef, err := sobject.ExMountSharedObject(ctx, b, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return so, func() { mountRef.Release() }
}

// waitPairedDevice waits until the account settings of so list peerID as a
// paired device.
func waitPairedDevice(ctx context.Context, t *testing.T, so sobject.SharedObject, peerID string) {
	// Watch the settings state.
	t.Helper()
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relStateCtr()

	// Stop at the first snapshot that lists the device.
	err = ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			settings, err := account_settings.ReadSnapshot(ctx, snap)
			if err != nil {
				return err
			}
			for _, d := range settings.GetPairedDevices() {
				if d.GetPeerId() == peerID {
					return io.EOF
				}
			}
			return nil
		},
		nil,
	)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}

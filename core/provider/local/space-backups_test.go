package provider_local

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/ulid"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	block_store_gdrive "github.com/s4wave/spacewave/db/block/store/gdrive"
	"github.com/s4wave/spacewave/db/block/store/gdrive/gdrivetest"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
)

// TestDriveBackupScheduleAndSecret exercises backend authorization, encrypted
// Secret storage, checkpoint coalescing and automatic-off over a real provider.
func TestDriveBackupScheduleAndSecret(t *testing.T) {
	// Route all Google API calls to isolated offline listeners with synthetic tokens.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	drive := gdrivetest.NewServer()
	defer drive.Close()
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"synthetic-access","token_type":"Bearer","expires_in":3600}`)
	}))
	defer oauth.Close()
	oauthURL, err := url.Parse(oauth.URL)
	if err != nil {
		t.Fatal(err)
	}
	driveURL, err := url.Parse(drive.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	original := http.DefaultTransport
	http.DefaultTransport = &googleTestTransport{base: original, oauth: oauthURL, drive: driveURL}
	defer func() { http.DefaultTransport = original }()
	t.Setenv("SPACEWAVE_GDRIVE_CLIENT_ID", "synthetic-client")
	t.Setenv("SPACEWAVE_GDRIVE_CLIENT_SECRET", "synthetic-client-setting")

	// Construct a provider and its production Space body/World controllers.
	tb, _, account, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	tb.StaticResolver.AddFactory(space_sobject.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(space_world.NewEngineFactory(tb.Bus, space_world.OpTypes{}))
	_, bodyRef, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&space_sobject.Config{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bodyRef.Release()
	_, public, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	entityPeer, err := peer.IDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	if err := account.commitAccountSettingsOps(ctx, &account_settings.AccountSettingsOp{Op: &account_settings.AccountSettingsOp_AddEntityKeypair{
		AddEntityKeypair: &session.EntityKeypair{PeerId: entityPeer.String(), AuthMethod: "pem"},
	}}); err != nil {
		t.Fatal(err)
	}

	// Add the folder and save its refresh token directly through the Secret API.
	client, err := block_store_gdrive.NewClient(drive.HTTP.Client(), "root", drive.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := client.CreateRootFolder(ctx, "fixture backup")
	if err != nil {
		t.Fatal(err)
	}
	backendID, check, err := account.AddGdriveStorageBackend(ctx, "drive", &account_settings.GdriveLocation{RootFolderId: folder.ID}, &block_store_gdrive.Credentials{RefreshToken: "synthetic-refresh"})
	if err != nil || backendID == "" || check.GetOutcome() != block_store_s3.CheckOutcome_CHECK_OUTCOME_OK {
		t.Fatalf("Drive backend registration: %v, %v", check, err)
	}
	settings, err := account.readAccountSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := account.ReadGdriveCredentials(ctx, settings.FindStorageBackend(backendID))
	if err != nil || stored.GetRefreshToken() != "synthetic-refresh" {
		t.Fatalf("Drive credential Secret: %v", err)
	}
	if _, err := account.ReadStorageCredentials(ctx, settings.FindStorageBackend(backendID)); !errors.Is(err, s4wave_secret.ErrSecretKindMismatch) {
		t.Fatalf("a refresh token could be decoded as an S3 access key: %v", err)
	}
	if err := account.SetDefaultStorageBackend(ctx, backendID); err == nil {
		t.Fatal("Drive backup became primary storage")
	}
	_, missing, err := account.AddGdriveStorageBackend(ctx, "missing-folder", &account_settings.GdriveLocation{RootFolderId: "missing"},
		&block_store_gdrive.Credentials{RefreshToken: "synthetic-refresh"})
	if err != nil || missing.GetOutcome() == block_store_s3.CheckOutcome_CHECK_OUTCOME_OK {
		t.Fatalf("an empty query falsely authenticated a missing Drive folder: %v, %v", missing, err)
	}

	// Reauthorization renews one Secret without changing the destination identity.
	credential := settings.FindStorageBackend(backendID).GetCredential().CloneVT()
	renewed, check, err := account.AddGdriveStorageBackend(ctx, "drive", &account_settings.GdriveLocation{RootFolderId: folder.ID},
		&block_store_gdrive.Credentials{RefreshToken: "synthetic-renewed-refresh"})
	if err != nil || renewed != backendID || check.GetOutcome() != block_store_s3.CheckOutcome_CHECK_OUTCOME_OK {
		t.Fatalf("Drive reauthorization: %v, %v", check, err)
	}
	settings, err = account.readAccountSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	backend := settings.FindStorageBackend(backendID)
	stored, err = account.ReadGdriveCredentials(ctx, backend)
	if err != nil || stored.GetRefreshToken() != "synthetic-renewed-refresh" || !credential.EqualVT(backend.GetCredential()) {
		t.Fatalf("reauthorization replaced the Secret or failed to renew it: %v", err)
	}

	// Create a checkpointed Space and an initially disabled backup.
	ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	seedProviderReplicaPayload(ctx, t, account, account, ref, "initial backup checkpoint")
	conf, err := account.SetSpaceBackup(ctx, &account_settings.SpaceBackup{
		SpaceId: ref.GetProviderResourceRef().GetId(), StorageBackendId: backendID, IntervalSeconds: 3, Automatic: true,
	})
	if err != nil || conf.GetAutomatic() {
		t.Fatalf("new backup did not default to off: %v", err)
	}
	repeated, err := account.SetSpaceBackup(ctx, &account_settings.SpaceBackup{SpaceId: conf.GetSpaceId(), StorageBackendId: backendID, Automatic: true})
	if err != nil || repeated.GetId() != conf.GetId() || repeated.GetAutomatic() {
		t.Fatalf("repeating add duplicated the target or enabled it: %v, %v", repeated, err)
	}

	// Another account on this same device must not load this configuration.
	otherInfo := account.t.accountInfo.CloneVT()
	otherInfo.ProviderAccountId = "another-account-on-the-same-volume"
	other := &ProviderAccount{vol: account.vol, t: &providerAccountTracker{accountInfo: otherInfo}}
	otherStore, err := other.spaceBackupRegistryStore()
	if err != nil {
		t.Fatal(err)
	}
	if leaked, _, err := nextSpaceBackupConfig(ctx, otherStore, nil); err != nil || leaked != nil {
		t.Fatalf("backup configuration escaped its account: %v, %v", leaked, err)
	}
	var backup *SpaceBackup
	account.backupsBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { backup = account.backups[conf.GetId()] })
	if backup.state.GetValue().GetIdle() == nil {
		t.Fatal("disabled new backup did not stay idle")
	}

	// A second Space shares the saved credential and the backend's one transport.
	sharedClient, err := account.openGdriveClient(ctx, backend)
	if err != nil {
		t.Fatal(err)
	}
	secondRef, err := account.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	seedProviderReplicaPayload(ctx, t, account, account, secondRef, "second Space using one Drive authorization")
	secondConf, err := account.SetSpaceBackup(ctx, &account_settings.SpaceBackup{
		SpaceId: secondRef.GetProviderResourceRef().GetId(), StorageBackendId: backendID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var secondResult *account_settings.SpaceBackupState
	if err := account.RunSpaceBackup(ctx, secondConf.GetId(), func(state *account_settings.SpaceBackupState) error {
		secondResult = state
		return nil
	}); err != nil || secondResult.GetUpToDate() == nil {
		t.Fatalf("second Space backup using the saved credential: %v, %v", secondResult, err)
	}
	reused, err := account.openGdriveClient(ctx, backend)
	if err != nil || reused != sharedClient {
		t.Fatalf("second Space opened a separate credential transport: %v", err)
	}

	// A registry watch replaces once, then sends only the changed backup ID.
	finishedWatch := errors.New("fixture received the registry delta")
	watchMessages := 0
	err = account.WatchSpaceBackups(ctx, func(list *account_settings.SpaceBackupList) error {
		watchMessages++
		if watchMessages == 1 {
			if len(list.GetBackups()) != 2 {
				return errors.New("initial registry snapshot omitted a backup")
			}
			changed := conf.CloneVT()
			changed.IntervalSeconds = 4
			_, err := account.SetSpaceBackup(ctx, changed)
			return err
		}
		if len(list.GetBackups()) != 1 || list.GetBackups()[0].GetBackup().GetId() != conf.GetId() {
			return errors.New("registry delta repeated an unchanged backup")
		}
		return finishedWatch
	})
	if !errors.Is(err, finishedWatch) || watchMessages != 2 {
		t.Fatalf("registry snapshot and delta: %d messages, %v", watchMessages, err)
	}

	// Enable deliberately, then wait for the first complete copy through its watch.
	conf.Automatic = true
	if _, err := account.SetSpaceBackup(ctx, conf); err != nil {
		t.Fatal(err)
	}
	first, err := backup.state.WaitValueWithValidator(ctx, func(state *account_settings.SpaceBackupState) (bool, error) {
		if failure := state.GetFailed(); failure != nil {
			return false, errors.New(failure.GetMessage())
		}
		return state.GetUpToDate() != nil, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	firstAt := time.Now()

	// Two checkpoints inside the interval produce one pass at its end.
	seedProviderReplicaPayload(ctx, t, account, account, ref, "first checkpoint inside interval")
	seedProviderReplicaPayload(ctx, t, account, account, ref, "second checkpoint inside interval")
	second, err := backup.state.WaitValueWithValidator(ctx, func(state *account_settings.SpaceBackupState) (bool, error) {
		if failure := state.GetFailed(); failure != nil {
			return false, errors.New(failure.GetMessage())
		}
		return state.GetUpToDate() != nil && state.GetRunSequence() > first.GetRunSequence(), nil
	}, nil)
	if err != nil || second.GetRunSequence() != first.GetRunSequence()+1 || time.Since(firstAt) < 2*time.Second {
		t.Fatalf("checkpoint coalescing: %v, %v", second, err)
	}

	// Turning automatic off leaves no source watcher or transfer after later checkpoints.
	conf.Automatic = false
	if _, err := account.SetSpaceBackup(ctx, conf); err != nil {
		t.Fatal(err)
	}
	before := len(drive.Calls())
	seedProviderReplicaPayload(ctx, t, account, account, ref, "checkpoint while automatic is off")
	window, endWindow := context.WithTimeout(ctx, 4*time.Second)
	defer endWindow()
	if calls, err := drive.WaitCalls(window, before); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("automatic-off backup called Drive: %v, %v", calls, err)
	}

	// A process restart preserves owner-action waits and an upload retry deadline.
	deadline := time.Now().Add(24 * time.Hour).Unix()
	waiting := &account_settings.SpaceBackupState{RunSequence: second.GetRunSequence(),
		State: &account_settings.SpaceBackupState_WaitingUploadLimit{WaitingUploadLimit: &account_settings.SpaceBackupWaiting{ResumeUnixSeconds: deadline}}}
	if err := account.saveBackupResult(ctx, conf.GetId(), waiting); err != nil {
		t.Fatal(err)
	}
	saved, err := account.readSpaceBackupState(ctx, conf.GetId())
	if err != nil {
		t.Fatal(err)
	}
	recovered := newSpaceBackup(conf, saved, func() {}, nil)
	if recovered.currentState().GetWaitingUploadLimit().GetResumeUnixSeconds() != deadline || recovered.requested != recovered.settled {
		t.Fatal("restart discarded the upload deadline or invented a running pass")
	}
	for _, call := range drive.Calls() {
		if call.Method == http.MethodDelete || call.Method == http.MethodPatch {
			t.Fatalf("backup deleted or changed a Drive object: %+v", call)
		}
	}
}

// TestBackupRequestTransitions keeps explicit requests alive while cancelling
// an automatic pass when its switch turns off, including a late progress event.
func TestBackupRequestTransitions(t *testing.T) {
	// A nil account lifetime lets this fixture test the actor's guarded state only.
	conf := &account_settings.SpaceBackup{Id: "backup", Automatic: true, IntervalSeconds: 3600}
	backup := newSpaceBackup(conf, nil, func() {}, nil)
	automatic := backup.request(false)
	conf.Automatic = false
	backup.configure(nil, conf)
	if backup.requested != backup.settled || backup.currentState().GetIdle() == nil {
		t.Fatal("disabling automatic work left its pass requested")
	}
	backup.publish(&account_settings.SpaceBackupState{RunSequence: automatic,
		State: &account_settings.SpaceBackupState_Copying{Copying: &account_settings.SpaceBackupProgress{}}}, false)
	if backup.currentState().GetIdle() == nil {
		t.Fatal("cancelled automatic progress revived its pass")
	}

	// A person joining an automatic pass makes the request independent of the switch.
	conf.Automatic = true
	backup.configure(nil, conf)
	joined := backup.request(false)
	if backup.request(true) != joined {
		t.Fatal("a concurrent manual command failed to join the active pass")
	}
	conf.Automatic = false
	backup.configure(nil, conf)
	if backup.requested == backup.settled || backup.currentState().GetCopying() == nil {
		t.Fatal("disabling automatic work cancelled an explicit manual request")
	}
}

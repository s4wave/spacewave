package provider_local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_gdrive "github.com/s4wave/spacewave/db/block/store/gdrive"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
)

// gdriveCredentialContentType distinguishes refresh tokens from access-key
// protobufs, whose first string field must never be sent to Google's token API.
const gdriveCredentialContentType = "application/x-protobuf; message=block.store.gdrive.Credentials"

// AddGdriveStorageBackend checks a backup folder by listing it, then writes its
// refresh token directly to the account's encrypted storage credential Secret.
// It never probes with a deletion or places a Space on this backend.
func (a *ProviderAccount) AddGdriveStorageBackend(
	ctx context.Context, name string, location *account_settings.GdriveLocation,
	creds *block_store_gdrive.Credentials,
) (string, *block_store_s3.CheckResult, error) {
	// Reject incomplete and duplicate names before any Drive request.
	if name == "" || location.GetRootFolderId() == "" || creds.GetRefreshToken() == "" {
		return "", nil, errors.New("Drive backend name, root folder and authorization are required")
	}
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return "", nil, err
	}
	existing := settings.FindStorageBackendByName(name)
	if existing != nil && existing.GetGdrive() == nil {
		return "", nil, errors.New("that storage backend name belongs to another storage type")
	}
	if existing != nil && existing.GetGdrive().GetRootFolderId() != location.GetRootFolderId() {
		return "", nil, errors.New("sign in to the existing Drive folder; create a new backend for a different folder")
	}

	// Listing is the entire Drive connectivity check; nothing is deleted.
	client, err := buildGdriveClient(ctx, location, creds)
	if err != nil {
		return "", nil, err
	}
	check := checkGdriveLocation(ctx, client)
	if check.GetOutcome() != block_store_s3.CheckOutcome_CHECK_OUTCOME_OK {
		return "", check, nil
	}

	// Save only the refresh token, using the existing Secret storage path.
	data, err := creds.MarshalVT()
	if err != nil {
		return "", nil, err
	}
	if existing == nil {
		id, err := a.saveStorageBackend(ctx, &account_settings.StorageBackend{DisplayName: name, Gdrive: location.CloneVT()}, data, false)
		return id, check, err
	}

	// Renew the same encrypted Secret, keeping the folder, proofs and backend ID.
	payload, err := s4wave_secret.ReadSecretPayload(ctx, a.t.p.b, existing.GetCredential())
	if err != nil {
		return "", nil, err
	}
	payload.Value, payload.ContentType = data, gdriveCredentialContentType
	payload.Version++
	payload.UpdatedAt = timestamppb.New(time.Now())
	if err := s4wave_secret.StoreSecretPayload(ctx, a.t.p.b, existing.GetCredential().GetRef(), payload); err != nil {
		return "", nil, err
	}
	return existing.GetId(), check, nil
}

// buildGdriveClient opens a lazy authenticated transport for one saved folder.
func buildGdriveClient(ctx context.Context, location *account_settings.GdriveLocation, creds *block_store_gdrive.Credentials) (*block_store_gdrive.Client, error) {
	// The installed app's client settings are filled in only by its operator.
	conf, err := block_store_gdrive.OAuthConfig()
	if err != nil {
		return nil, err
	}
	httpClient, err := block_store_gdrive.AuthorizedHTTP(ctx, conf, creds)
	if err != nil {
		return nil, err
	}
	return block_store_gdrive.NewClient(httpClient, location.GetRootFolderId(), "")
}

// checkGdriveLocation proves listing access without writing or deleting a probe.
func checkGdriveLocation(ctx context.Context, client *block_store_gdrive.Client) *block_store_s3.CheckResult {
	// Stop after one object; an empty folder is also a successful listing.
	err := client.CheckRootFolder(ctx)
	if err == nil {
		err = client.ListObjects(ctx, "", "", func(string, int64) error { return errCheckListed })
	}
	outcome := block_store_s3.CheckOutcome_CHECK_OUTCOME_OK
	if err != nil && !errors.Is(err, errCheckListed) {
		outcome = block_store_s3.CheckOutcome_CHECK_OUTCOME_FAILED
		if errors.Is(err, block_store_gdrive.ErrSignIn) {
			outcome = block_store_s3.CheckOutcome_CHECK_OUTCOME_CREDENTIALS_REJECTED
		}
	}
	return &block_store_s3.CheckResult{Outcome: outcome}
}

// ReadGdriveCredentials decodes a Drive backend's storage credential Secret.
func (a *ProviderAccount) ReadGdriveCredentials(ctx context.Context, backend *account_settings.StorageBackend) (*block_store_gdrive.Credentials, error) {
	// Reject another credential kind before decrypting its payload.
	if backend.GetGdrive() == nil || backend.GetCredential().GetKind() != s4wave_secret.SecretKindStorageCredential {
		return nil, s4wave_secret.ErrSecretKindMismatch
	}
	payload, err := s4wave_secret.ReadSecretPayload(ctx, a.t.p.b, backend.GetCredential())
	if err != nil {
		return nil, err
	}
	if payload.GetContentType() != gdriveCredentialContentType {
		return nil, s4wave_secret.ErrSecretKindMismatch
	}

	// Keep the decoded credential in memory only, never in status or logs.
	creds := &block_store_gdrive.Credentials{}
	if err := creds.UnmarshalVT(payload.GetValue()); err != nil {
		return nil, errors.New("unable to decode Drive storage credential")
	}
	return creds, nil
}

// driveConnection binds a cached client to its exact folder and credential reference.
type driveConnection struct {
	// identity names the folder and encrypted credential object, without a token.
	identity string
	// client shares the authorization transport and pacer among this backend's backups.
	client *block_store_gdrive.Client
}

// openGdriveClient shares one pacer and OAuth transport per saved credential.
func (a *ProviderAccount) openGdriveClient(ctx context.Context, backend *account_settings.StorageBackend) (*block_store_gdrive.Client, error) {
	// Join an existing credential load while keeping cancellation local to this waiter.
	credentials, err := a.ReadGdriveCredentials(ctx, backend)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(credentials.GetRefreshToken()))
	identity := backend.GetGdrive().GetRootFolderId() + "/" + backend.GetCredential().GetNestedSharedObjectId() + "/" + hex.EncodeToString(digest[:])
	result := a.driveLoads.DoChan(backend.GetId()+"/"+identity, func() (any, error) {
		// Reuse the credential's existing transport without holding its cache lock.
		a.driveClientsMtx.Lock()
		cached := a.driveClients[backend.GetId()]
		a.driveClientsMtx.Unlock()
		if cached.client != nil && cached.identity == identity {
			return cached.client, nil
		}

		// Decrypt through the existing Secret API, outside every cache lock.
		client, err := buildGdriveClient(a.lifecycleCtx, backend.GetGdrive(), credentials)
		if err != nil {
			return nil, err
		}

		// Replace only this backend's old transport after construction succeeded.
		a.driveClientsMtx.Lock()
		a.driveClients[backend.GetId()] = driveConnection{identity: identity, client: client}
		a.driveClientsMtx.Unlock()
		return client, nil
	})
	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case value := <-result:
		if value.Err != nil {
			return nil, value.Err
		}
		return value.Val.(*block_store_gdrive.Client), nil
	}
}

package provider_local

import (
	"context"
	"strings"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
)

// AddStorageBackend checks the bucket with the credentials and, when the check
// passes, keeps the credentials in a storage credential Secret owned by the
// account and records the backend in the account settings. It returns the new
// backend id, or an empty id when the check failed.
func (a *ProviderAccount) AddStorageBackend(
	ctx context.Context,
	displayName string,
	location *account_settings.S3Location,
	creds *block_store_s3.Credentials,
	setDefault bool,
) (string, *block_store_s3.CheckResult, error) {
	// Reject an incomplete location or a taken name before touching the bucket.
	if displayName == "" {
		return "", nil, errors.New("display name is required")
	}
	if err := location.Validate(); err != nil {
		return "", nil, err
	}
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return "", nil, err
	}
	if settings.FindStorageBackendByName(displayName) != nil {
		return "", nil, errors.Errorf("a storage backend named %q already exists", displayName)
	}

	// Save nothing unless the bucket accepts a probe object.
	check := CheckS3Location(ctx, location, creds)
	if check.GetOutcome() != block_store_s3.CheckOutcome_CHECK_OUTCOME_OK {
		return "", check, nil
	}

	// Keep the credentials in a Secret the account's Sessions can decrypt.
	credData, err := creds.MarshalVT()
	if err != nil {
		return "", nil, err
	}
	credential, err := s4wave_secret.CreateSecretObject(ctx, a.t.p.b, a, s4wave_secret.CreateSecretOptions{
		DisplayName: displayName,
		Kind:        s4wave_secret.SecretKindStorageCredential,
		ContentType: s4wave_secret.StorageCredentialContentType,
		Value:       credData,
	})
	if err != nil {
		return "", nil, errors.Wrap(err, "create storage credential")
	}

	// Record the backend, and remove the Secret if the settings reject it.
	backend := &account_settings.StorageBackend{
		Id:          ulid.NewULID(),
		DisplayName: displayName,
		S3:          location.CloneVT(),
		Credential:  credential,
	}
	ops := []*account_settings.AccountSettingsOp{{
		Op: &account_settings.AccountSettingsOp_UpsertStorageBackend{UpsertStorageBackend: backend},
	}}
	if setDefault {
		ops = append(ops, &account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_SetDefaultStorageBackend{
				SetDefaultStorageBackend: &account_settings.SetDefaultStorageBackendOp{StorageBackendId: backend.GetId()},
			},
		})
	}
	if err := a.commitAccountSettingsOps(ctx, ops...); err != nil {
		if delErr := a.DeleteSharedObject(ctx, credential.GetNestedSharedObjectId()); delErr != nil {
			a.le.WithError(delErr).Warn("unable to delete unused storage credential")
		}
		return "", nil, err
	}
	return backend.GetId(), check, nil
}

// CheckStorageBackend runs the connectivity check against a saved backend.
func (a *ProviderAccount) CheckStorageBackend(ctx context.Context, backendID string) (*block_store_s3.CheckResult, error) {
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return nil, err
	}
	backend := settings.FindStorageBackend(backendID)
	if backend == nil {
		return nil, account_settings.ErrStorageBackendNotFound
	}
	creds, err := a.ReadStorageCredentials(ctx, backend)
	if err != nil {
		return nil, err
	}
	return CheckS3Location(ctx, backend.GetS3(), creds), nil
}

// ReadStorageCredentials decrypts the credentials of a storage backend.
func (a *ProviderAccount) ReadStorageCredentials(
	ctx context.Context,
	backend *account_settings.StorageBackend,
) (*block_store_s3.Credentials, error) {
	credential := backend.GetCredential()
	if credential.GetKind() != s4wave_secret.SecretKindStorageCredential {
		return nil, s4wave_secret.ErrSecretKindMismatch
	}
	payload, err := s4wave_secret.ReadSecretPayload(ctx, a.t.p.b, credential)
	if err != nil {
		return nil, errors.Wrap(err, "read storage credential")
	}
	creds := &block_store_s3.Credentials{}
	if err := creds.UnmarshalVT(payload.GetValue()); err != nil {
		return nil, errors.Wrap(err, "decode storage credential")
	}
	return creds, nil
}

// RemoveStorageBackend removes a backend that holds no Space and deletes its
// credential Secret. While Spaces are placed on it, the error names them.
func (a *ProviderAccount) RemoveStorageBackend(ctx context.Context, backendID string) error {
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return err
	}
	backend := settings.FindStorageBackend(backendID)
	if backend == nil {
		return account_settings.ErrStorageBackendNotFound
	}
	if placed := settings.PlacedBlockStoreIDs(backendID); len(placed) != 0 {
		names := make([]string, len(placed))
		for i, blockStoreID := range placed {
			names[i] = blockStoreID
			if _, name := settings.FindSpaceByBlockStore(blockStoreID); name != "" {
				names[i] = name
			}
		}
		return errors.Wrap(account_settings.ErrStorageBackendInUse, strings.Join(names, ", "))
	}

	// Commit the removal before deleting the Secret so no replica keeps a
	// backend whose credentials are gone.
	if err := a.commitAccountSettingsOps(ctx, &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemoveStorageBackend{
			RemoveStorageBackend: &account_settings.RemoveStorageBackendOp{StorageBackendId: backendID},
		},
	}); err != nil {
		return err
	}
	err = a.DeleteSharedObject(ctx, backend.GetCredential().GetNestedSharedObjectId())
	if err != nil && !errors.Is(err, sobject.ErrSharedObjectNotFound) {
		return errors.Wrap(err, "delete storage credential")
	}
	return nil
}

// SetDefaultStorageBackend selects the backend new Spaces use, or the
// account's own storage when backendID is empty.
func (a *ProviderAccount) SetDefaultStorageBackend(ctx context.Context, backendID string) error {
	return a.commitAccountSettingsOps(ctx, &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_SetDefaultStorageBackend{
			SetDefaultStorageBackend: &account_settings.SetDefaultStorageBackendOp{StorageBackendId: backendID},
		},
	})
}

// commitAccountSettingsOps commits each op to the account settings in order.
func (a *ProviderAccount) commitAccountSettingsOps(ctx context.Context, ops ...*account_settings.AccountSettingsOp) error {
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}
	so, release, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}
	defer release()
	for _, op := range ops {
		if err := commitAccountSettingsOp(ctx, so, op); err != nil {
			return err
		}
	}
	return nil
}

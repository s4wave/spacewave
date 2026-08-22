package s4wave_sshhost

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
)

// SshHostCredentialSecretExpectation describes one expected Secret reference.
type SshHostCredentialSecretExpectation struct {
	Field     string
	ObjectKey string
	Kind      string
}

// SshHostCredentialSecretExpectations returns the expected Secret kind for each set credential ref.
func SshHostCredentialSecretExpectations(refs *SshHostCredentialRefs) []SshHostCredentialSecretExpectation {
	if refs == nil {
		return nil
	}
	expectations := make([]SshHostCredentialSecretExpectation, 0, 3)
	if key := refs.GetPrivateKeySecretObjectKey(); key != "" {
		expectations = append(expectations, SshHostCredentialSecretExpectation{
			Field:     "private_key_secret_object_key",
			ObjectKey: key,
			Kind:      s4wave_secret.SecretKindSSHPrivateKey,
		})
	}
	if key := refs.GetPasswordSecretObjectKey(); key != "" {
		expectations = append(expectations, SshHostCredentialSecretExpectation{
			Field:     "password_secret_object_key",
			ObjectKey: key,
			Kind:      s4wave_secret.SecretKindSSHPassword,
		})
	}
	if key := refs.GetPassphraseSecretObjectKey(); key != "" {
		expectations = append(expectations, SshHostCredentialSecretExpectation{
			Field:     "passphrase_secret_object_key",
			ObjectKey: key,
			Kind:      s4wave_secret.SecretKindSSHPassphrase,
		})
	}
	return expectations
}

// ValidateSshHostCredentialSecrets checks SSH Host refs against redacted Secret metadata.
func ValidateSshHostCredentialSecrets(ctx context.Context, ws world.WorldState, refs *SshHostCredentialRefs) error {
	for _, exp := range SshHostCredentialSecretExpectations(refs) {
		if err := world_types.CheckObjectType(ctx, ws, exp.ObjectKey, s4wave_secret.SecretTypeID); err != nil {
			return errors.Wrapf(err, "ssh host credential %s", exp.Field)
		}
		secret, err := world.LookupObjectBody[*s4wave_secret.Secret](
			ctx,
			ws,
			exp.ObjectKey,
			s4wave_secret.NewSecretBlock,
		)
		if err != nil {
			return errors.Wrapf(err, "ssh host credential %s", exp.Field)
		}
		if secret.GetKind() != exp.Kind {
			return errors.Errorf(
				"ssh host credential %s references secret kind %q, want %q",
				exp.Field,
				secret.GetKind(),
				exp.Kind,
			)
		}
	}
	return nil
}

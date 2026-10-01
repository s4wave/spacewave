package provider_spacewave

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// accountMigrationDomain binds an independent account proof to the binary transition.
const accountMigrationDomain = "spacewave 2026-09-10 account migration v1."

// MergePairingAccount preserves both providers' resource and Session authority.
func (a *ProviderAccount) MergePairingAccount(ctx context.Context, mounted session.Session, destination provider.ProviderAccount, ref *session.SessionRef) (func(context.Context) (*session.SessionRef, error), error) {
	return provider_migration.Merge(ctx, a, mounted, destination, ref)
}

// migrationClient signs with the exact Session participating in the approval.
func (a *ProviderAccount) migrationClient(key crypto.PrivKey) (*SessionClient, error) {
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return nil, err
	}
	return NewSessionClient(a.p.httpCli, a.p.endpoint, a.p.GetSigningEnvPrefix(), key, id.String()), nil
}

// MigrationInfo uses the provider's authoritative Session set and settings binding.
func (a *ProviderAccount) MigrationInfo(ctx context.Context, key crypto.PrivKey) (*provider_migration.Info, error) {
	// Read the account behind the migration key and require that it is this one.
	client, err := a.migrationClient(key)
	if err != nil {
		return nil, err
	}
	info, err := client.GetAccountInfo(ctx)
	if err != nil {
		return nil, err
	}
	if info.GetAccountId() != a.accountID {
		return nil, errors.New("this Session already belongs to another account")
	}

	// Read the settings binding and the account's Sessions.
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := client.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	// Describe the account, its Session peers and any transition it started.
	result := &provider_migration.Info{
		Settings:            ref,
		Endpoint:            a.p.endpoint,
		ParticipantEntity:   a.accountID,
		ParticipantUsername: info.GetEntityId(),
	}
	for _, row := range rows {
		result.SessionPeers = append(result.SessionPeers, row.GetPeerId())
	}
	result.ParticipantPeers = slices.Clone(result.SessionPeers)
	if transition := info.GetTransition(); transition.GetSource().GetProviderAccountId() == a.accountID {
		result.Transition = transition.CloneVT()
	}
	return result, nil
}

// CheckMigrationSessions refuses conflicts and capacity overflow before data movement.
func (a *ProviderAccount) CheckMigrationSessions(ctx context.Context, transition *provider.AccountTransition, source, destination crypto.PrivKey) error {
	return a.acceptMigration(ctx, transition, source, destination, true)
}

// AcceptMigrationSessions commits one explicit cross-account authorization.
func (a *ProviderAccount) AcceptMigrationSessions(ctx context.Context, transition *provider.AccountTransition, source, destination crypto.PrivKey) error {
	if err := a.acceptMigration(ctx, transition, source, destination, false); err != nil {
		return err
	}
	a.BumpLocalEpoch()
	return nil
}

func (a *ProviderAccount) acceptMigration(ctx context.Context, transition *provider.AccountTransition, source, destination crypto.PrivKey, checkOnly bool) error {
	if err := transition.Validate(); err != nil {
		return err
	}
	if transition.GetDestination().GetProviderId() != a.GetProviderID() || transition.GetDestination().GetProviderAccountId() != a.accountID || transition.GetDestinationEndpoint() != a.p.endpoint {
		return errors.New("account migration names another destination provider")
	}
	if endpoint := transition.GetSourceEndpoint(); endpoint != "" && endpoint != a.p.endpoint {
		return errors.New("cloud accounts must use the same configured provider to merge")
	}
	client, err := a.migrationClient(destination)
	if err != nil {
		return err
	}
	signer, signature, err := signAccountTransition(transition, source)
	if err != nil {
		return err
	}
	body, err := (&api.AccountMigrationRequest{Transition: transition, SourcePeerId: signer, SourceSignature: signature, CheckOnly: checkOnly}).MarshalVT()
	if err != nil {
		return err
	}
	return client.submitAccountTransition(ctx, "/api/account/migration", body, transition)
}

// CommitAccountTransition verifies cloud reattachment or publishes a local
// destination's independent receipt while retaining the source recovery account.
func (a *ProviderAccount) CommitAccountTransition(ctx context.Context, transition *provider.AccountTransition, source, destination crypto.PrivKey) error {
	client, err := a.migrationClient(source)
	if err != nil {
		return err
	}
	if transition.GetDestinationEndpoint() != "" {
		info, err := client.GetAccountInfo(ctx)
		if err != nil {
			return err
		}
		if info.GetAccountId() != transition.GetDestination().GetProviderAccountId() || !info.GetTransition().EqualVT(transition) {
			return errors.New("cloud provider did not retain the approved account transition")
		}
		return nil
	}
	signer, signature, err := signAccountTransition(transition, destination)
	if err != nil {
		return err
	}
	body, err := (&api.AccountDepartureRequest{Transition: transition, DestinationPeerId: signer, DestinationSignature: signature}).MarshalVT()
	if err != nil {
		return err
	}
	if err := client.submitAccountTransition(ctx, "/api/account/departure", body, transition); err != nil {
		return err
	}
	a.BumpLocalEpoch()
	return nil
}

func signAccountTransition(transition *provider.AccountTransition, key crypto.PrivKey) (string, []byte, error) {
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return "", nil, err
	}
	data, err := transition.MarshalVT()
	if err != nil {
		return "", nil, err
	}
	signature, err := key.Sign(append([]byte(accountMigrationDomain), data...))
	return id.String(), signature, err
}

// submitAccountTransition requires the provider to echo the exact durable decision.
func (c *SessionClient) submitAccountTransition(ctx context.Context, path string, body []byte, expected *provider.AccountTransition) error {
	data, err := c.doPostBinary(ctx, path, body, nil, SeedReasonMutation)
	if err != nil {
		return err
	}
	var result provider.AccountTransition
	if err := result.UnmarshalVT(data); err != nil {
		return err
	}
	if !result.EqualVT(expected) {
		return errors.New("cloud provider acknowledged another account transition")
	}
	return nil
}

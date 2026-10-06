//go:build e2e

package onboarding_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/ulid"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func accessSessionClient(ctx context.Context, t *testing.T, accountID string) *provider_spacewave.SessionClient {
	t.Helper()

	prov, provRef, err := provider.ExLookupProvider(ctx, env.tb.Bus, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()

	swProv := prov.(*provider_spacewave.Provider)
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)

	swAcc := accIface.(*provider_spacewave.ProviderAccount)
	return swAcc.GetSessionClient()
}

func createOrganization(t *testing.T, cli *provider_spacewave.SessionClient, ctx context.Context) string {
	t.Helper()

	createResp, err := cli.CreateOrganization(ctx, "Security Org "+ulid.NewULID())
	if err != nil {
		t.Fatal(err)
	}
	var org api.OrgResponse
	if err := org.UnmarshalVT(createResp); err != nil {
		t.Fatal(err)
	}
	if org.GetId() == "" {
		t.Fatal("organization response missing id")
	}
	return org.GetId()
}

func inviteAndJoinMember(
	t *testing.T,
	ctx context.Context,
	ownerCli *provider_spacewave.SessionClient,
	memberCli *provider_spacewave.SessionClient,
	orgID string,
) {
	t.Helper()

	inviteResp, err := ownerCli.CreateOrgInvite(ctx, orgID, "link", 1, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	var invite api.OrgInviteResponse
	if err := invite.UnmarshalVT(inviteResp); err != nil {
		t.Fatal(err)
	}
	if invite.GetToken() == "" {
		t.Fatal("invite response missing token")
	}

	if _, err := memberCli.JoinOrganization(ctx, invite.GetToken()); err != nil {
		t.Fatal(err)
	}
}

func assignOwnerBillingToOrg(
	t *testing.T,
	ctx context.Context,
	ownerCli *provider_spacewave.SessionClient,
	orgID string,
) {
	t.Helper()

	baID, err := ownerCli.CreateBillingAccount(ctx, "Security Org Billing "+ulid.NewULID())
	if err != nil {
		t.Fatal(err)
	}
	setTestBillingSubscriptionStatus(t, baID, "active")
	if _, err := ownerCli.AssignBillingAccount(ctx, baID, "organization", orgID); err != nil {
		t.Fatal(err)
	}
}

func setTestBillingSubscriptionStatus(t *testing.T, billingAccountID, status string) {
	// Set the status through the coordinator test helper.
	t.Helper()
	var a fastjson.Arena
	body := a.NewObject()
	body.Set("billing_account_id", a.NewString(billingAccountID))
	body.Set("subscription_status", a.NewString(status))
	postTestHelper(context.Background(), t, "/api/test/set-billing-subscription", body)
}

func TestBillingSelfServiceOwnership(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	// Open two unrelated cloud accounts and their session clients.
	a := createCloudSession(ctx, t)
	b := createCloudSession(ctx, t)
	cliA := accessSessionClient(ctx, t, a.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())
	cliB := accessSessionClient(ctx, t, b.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())

	// Give each account its own billing account.
	baA, err := cliA.CreateBillingAccount(ctx, "Billing A "+ulid.NewULID())
	if err != nil {
		t.Fatal(err)
	}
	baB, err := cliB.CreateBillingAccount(ctx, "Billing B "+ulid.NewULID())
	if err != nil {
		t.Fatal(err)
	}

	// The owner reads its own billing state.
	if _, err := cliA.GetBillingState(ctx, baA); err != nil {
		t.Fatal(err)
	}

	// The coordinator refuses a read of the other account's billing state.
	_, err = cliA.GetBillingState(ctx, baB)
	if err == nil || !strings.Contains(err.Error(), "billing_access_denied") {
		t.Fatalf("expected billing_access_denied for foreign billing state, got %v", err)
	}

	// The coordinator refuses to cancel the other account's subscription.
	_, err = cliA.CancelSubscription(ctx, baB)
	if err == nil || !strings.Contains(err.Error(), "billing_access_denied") {
		t.Fatalf("expected billing_access_denied for foreign billing cancel, got %v", err)
	}
}

func TestOrgOwnedCreateRequiresManageSpaces(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	owner := createCloudSession(ctx, t)
	member := createCloudSession(ctx, t)
	outsider := createCloudSession(ctx, t)

	ownerCli := accessSessionClient(ctx, t, owner.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())
	memberCli := accessSessionClient(ctx, t, member.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())
	outsiderCli := accessSessionClient(ctx, t, outsider.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())

	orgID := createOrganization(t, ownerCli, ctx)
	assignOwnerBillingToOrg(t, ctx, ownerCli, orgID)
	inviteAndJoinMember(t, ctx, ownerCli, memberCli, orgID)

	memberSO := ulid.NewULID()
	if err := memberCli.CreateSharedObject(ctx, memberSO, "member-owned", "space", "organization", orgID, false); err != nil {
		t.Fatalf("member org-owned create failed: %v", err)
	}

	outsiderSO := ulid.NewULID()
	err := outsiderCli.CreateSharedObject(ctx, outsiderSO, "outsider-owned", "space", "organization", orgID, false)
	if err == nil || !strings.Contains(err.Error(), "rbac_denied") {
		t.Fatalf("expected rbac_denied for outsider org-owned create, got %v", err)
	}
}

func TestSharedObjectMetadataRequiresReadAccess(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	// Open the owner's and an outsider's cloud accounts.
	owner := createCloudSession(ctx, t)
	outsider := createCloudSession(ctx, t)
	ownerCli := accessSessionClient(ctx, t, owner.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())
	outsiderCli := accessSessionClient(ctx, t, outsider.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())

	// Subscribe the owner and create its Space.
	subscribeCloudAccount(ctx, t, owner.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())
	soID := ulid.NewULID()
	if err := ownerCli.CreateSharedObject(ctx, soID, "secret", "space", "", "", false); err != nil {
		t.Fatal(err)
	}

	// The owner reads the Space metadata.
	metaData, err := ownerCli.GetSOMetadata(ctx, soID)
	if err != nil {
		t.Fatal(err)
	}
	var meta api.SpaceMetadataResponse
	if err := meta.UnmarshalVT(metaData); err != nil {
		t.Fatal(err)
	}
	if meta.GetDisplayName() != "secret" {
		t.Fatalf("unexpected metadata display name: %q", meta.GetDisplayName())
	}

	// The coordinator refuses the outsider's metadata read.
	_, err = outsiderCli.GetSOMetadata(ctx, soID)
	if err == nil || !strings.Contains(err.Error(), "rbac_denied") {
		t.Fatalf("expected rbac_denied for foreign metadata read, got %v", err)
	}
}

func TestTransferRequiresResourceTransferAndOrgManageSpaces(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	// Open the owner's and a member's cloud accounts.
	owner := createCloudSession(ctx, t)
	member := createCloudSession(ctx, t)
	ownerCli := accessSessionClient(ctx, t, owner.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())
	memberCli := accessSessionClient(ctx, t, member.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())

	// Create an organization the member joins.
	orgID := createOrganization(t, ownerCli, ctx)
	inviteAndJoinMember(t, ctx, ownerCli, memberCli, orgID)

	// Subscribe the owner and create its personal Space.
	subscribeCloudAccount(ctx, t, owner.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())
	soID := ulid.NewULID()
	if err := ownerCli.CreateSharedObject(ctx, soID, "transfer-me", "space", "", "", false); err != nil {
		t.Fatal(err)
	}

	// The coordinator refuses a transfer by the member, who does not control the Space.
	_, err := memberCli.TransferResource(ctx, soID, "organization", orgID)
	if err == nil || !strings.Contains(err.Error(), "rbac_denied") {
		t.Fatalf("expected rbac_denied for member transfer without source control, got %v", err)
	}

	// The owner transfers the Space to the organization.
	if _, err := ownerCli.TransferResource(ctx, soID, "organization", orgID); err != nil {
		t.Fatalf("owner transfer failed: %v", err)
	}
}

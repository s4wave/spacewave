//go:build e2e

package onboarding_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

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
	t.Helper()

	body := `{"billing_account_id":"` + billingAccountID + `","subscription_status":"` + status + `"}`
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		env.cloudURL+"/api/test/set-billing-subscription",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set billing subscription status returned %d", resp.StatusCode)
	}
}

func TestBillingSelfServiceOwnership(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	a := createCloudSession(ctx, t)
	b := createCloudSession(ctx, t)

	cliA := accessSessionClient(ctx, t, a.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())
	cliB := accessSessionClient(ctx, t, b.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())

	infoA, err := cliA.GetAccountInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	infoB, err := cliB.GetAccountInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if infoA.GetBillingAccountId() == "" || infoB.GetBillingAccountId() == "" {
		t.Fatal("expected billing account ids for both accounts")
	}

	if _, err := cliA.GetBillingState(ctx, infoA.GetBillingAccountId()); err != nil {
		t.Fatal(err)
	}

	_, err = cliA.GetBillingState(ctx, infoB.GetBillingAccountId())
	if err == nil || !strings.Contains(err.Error(), "billing_access_denied") {
		t.Fatalf("expected billing_access_denied for foreign billing state, got %v", err)
	}

	_, err = cliA.CancelSubscription(ctx, infoB.GetBillingAccountId())
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
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	owner := createCloudSession(ctx, t)
	outsider := createCloudSession(ctx, t)

	ownerCli := accessSessionClient(ctx, t, owner.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())
	outsiderCli := accessSessionClient(ctx, t, outsider.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())

	soID := ulid.NewULID()
	if err := ownerCli.CreateSharedObject(ctx, soID, "secret", "space", "", "", false); err != nil {
		t.Fatal(err)
	}

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

	_, err = outsiderCli.GetSOMetadata(ctx, soID)
	if err == nil || !strings.Contains(err.Error(), "rbac_denied") {
		t.Fatalf("expected rbac_denied for foreign metadata read, got %v", err)
	}
}

func TestTransferRequiresResourceTransferAndOrgManageSpaces(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	owner := createCloudSession(ctx, t)
	member := createCloudSession(ctx, t)

	ownerCli := accessSessionClient(ctx, t, owner.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())
	memberCli := accessSessionClient(ctx, t, member.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())

	orgID := createOrganization(t, ownerCli, ctx)
	inviteAndJoinMember(t, ctx, ownerCli, memberCli, orgID)

	soID := ulid.NewULID()
	if err := ownerCli.CreateSharedObject(ctx, soID, "transfer-me", "space", "", "", false); err != nil {
		t.Fatal(err)
	}

	_, err := memberCli.TransferResource(ctx, soID, "organization", orgID)
	if err == nil || !strings.Contains(err.Error(), "rbac_denied") {
		t.Fatalf("expected rbac_denied for member transfer without source control, got %v", err)
	}

	if _, err := ownerCli.TransferResource(ctx, soID, "organization", orgID); err != nil {
		t.Fatalf("owner transfer failed: %v", err)
	}
}

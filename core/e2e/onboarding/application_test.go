//go:build e2e

package onboarding_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/ulid"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// TestApplicationRegistration exercises signed registration and operator readback
// through mounted provider accounts against an isolated Worker database.
func TestApplicationRegistration(t *testing.T) {
	// Retain independent administrator and operator Sessions for the whole flow.
	ctx, cancel := context.WithTimeout(env.ctx, 2*time.Minute)
	t.Cleanup(cancel)
	admin := createCloudSession(ctx, t)
	operator := createCloudSession(ctx, t)
	adminID := admin.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()
	operatorID := operator.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()
	adminClient := accessSessionClient(ctx, t, adminID)
	operatorClient := accessSessionClient(ctx, t, operatorID)

	// Approve only the administrator using the local test deployment's helper.
	var arena fastjson.Arena
	grant := arena.NewObject()
	grant.Set("account_id", arena.NewString(adminID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, env.cloudURL+"/api/test/set-admin", bytes.NewReader(grant.MarshalTo(nil)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("grant test administration: HTTP %d", resp.StatusCode)
	}

	// Register without accepting a payer or granting active application access.
	input := &api.RegisterApplicationRequest{
		DisplayName:       "Application integration",
		OperatorAccountId: operatorID,
		RequestId:         ulid.NewULID(),
	}
	registered, err := adminClient.RegisterApplication(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	app := registered.GetApplication()
	if app.GetId() == "" {
		t.Fatal("registration returned no application")
	}
	if app.GetState() != api.ApplicationState_APPLICATION_STATE_PAUSED {
		t.Fatalf("registration state: %v", app.GetState())
	}
	if app.GetBillingAccountId() != "" {
		t.Fatal("registration accepted funding without payer consent")
	}

	// Retry through the network and read the same registration as its operator.
	retried, err := adminClient.RegisterApplication(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !registered.EqualVT(retried) {
		t.Fatal("registration retry changed the persisted application")
	}
	readback, err := operatorClient.GetApplication(ctx, app.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if !app.EqualVT(readback.GetApplication()) {
		t.Fatal("operator readback changed the application")
	}

	// Operator control does not authorize approving another application.
	if _, err := operatorClient.RegisterApplication(ctx, input); err == nil {
		t.Fatal("operator registered an application without platform administration")
	}

	// Accept an operator-owned payer through the signed application boundary.
	payerID, err := operatorClient.CreateBillingAccount(ctx, "Application integration payer")
	if err != nil {
		t.Fatal(err)
	}
	fundingRequest := &api.SetApplicationFundingRequest{
		ApplicationId:    app.GetId(),
		BillingAccountId: payerID,
		Funding:          api.ApplicationFunding_APPLICATION_FUNDING_OPERATOR,
		ExpectedRevision: app.GetRevision(),
	}
	funded, err := operatorClient.SetApplicationFunding(ctx, fundingRequest)
	if err != nil {
		t.Fatal(err)
	}
	if funded.GetAssignment().GetBillingAccountId() != payerID {
		t.Fatal("funding returned a different payer")
	}
	retryFunding, err := operatorClient.SetApplicationFunding(ctx, fundingRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !funded.EqualVT(retryFunding) {
		t.Fatal("funding retry changed the accepted assignment")
	}

	// Read accepted history through the generated Go-to-Worker response codec.
	history, err := operatorClient.ListApplicationFunding(ctx, &api.ListApplicationFundingRequest{
		ApplicationId: app.GetId(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.GetAssignments()) != 1 || !history.GetAssignments()[0].EqualVT(funded.GetAssignment()) {
		t.Fatal("funding history does not contain the accepted assignment")
	}

	// Activate through the real lifecycle API before enrolling independent credentials.
	changeState := func(revision uint64, state api.ApplicationState) {
		t.Helper()
		_, err := operatorClient.SetApplicationState(ctx, &api.SetApplicationStateRequest{
			ApplicationId: app.GetId(), ExpectedRevision: revision, State: state,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	changeState(2, api.ApplicationState_APPLICATION_STATE_ACTIVE)
	first, accountID := enrollApplicationTestSession(ctx, t, operatorClient, app.GetId(), 3)
	second, recoveredID := enrollApplicationTestSession(ctx, t, operatorClient, app.GetId(), 3)
	if accountID != recoveredID {
		t.Fatal("independent credential recovered a different account")
	}
	for _, client := range []*provider_spacewave.SessionClient{first, second} {
		if _, err := client.GetAccountInfo(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Paused Sessions survive, and disabling cannot resurrect them on reactivation.
	changeState(3, api.ApplicationState_APPLICATION_STATE_PAUSED)
	if _, err := first.GetAccountInfo(ctx); err == nil {
		t.Fatal("paused application admitted an account operation")
	}
	changeState(4, api.ApplicationState_APPLICATION_STATE_ACTIVE)
	if _, err := first.GetAccountInfo(ctx); err != nil {
		t.Fatal(err)
	}
	changeState(5, api.ApplicationState_APPLICATION_STATE_DISABLED)
	changeState(6, api.ApplicationState_APPLICATION_STATE_ACTIVE)
	if _, err := first.GetAccountInfo(ctx); err == nil {
		t.Fatal("reactivation restored a disabled Session")
	}
	third, reactivatedID := enrollApplicationTestSession(ctx, t, operatorClient, app.GetId(), 7)
	if reactivatedID != accountID {
		t.Fatal("fresh login after disable lost the managed account")
	}
	if _, err := third.GetAccountInfo(ctx); err != nil {
		t.Fatal(err)
	}
}

// enrollApplicationTestSession independently signs a fresh login credential and
// registers its Session through the ordinary provider API. The test operator
// stands in for a trusted identity integration; no external login is simulated.
func enrollApplicationTestSession(ctx context.Context, t *testing.T, operator *provider_spacewave.SessionClient, applicationID string, revision uint64) (*provider_spacewave.SessionClient, string) {
	t.Helper()
	entityKey, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	entityID, err := peer.IDFromPrivateKey(entityKey)
	if err != nil {
		t.Fatal(err)
	}
	entity := provider_spacewave.NewEntityClientDirect(httpClient, env.cloudURL, "", entityKey, entityID)
	request, err := entity.SignManagedAccountEnrollment(ctx, &api.ManagedAccountEnrollment{
		ApplicationId:               applicationID,
		Issuer:                      "https://identity.example",
		Subject:                     "same-verified-subject",
		KeypairPeerId:               entityID.String(),
		ExpectedApplicationRevision: revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	enrolled, err := operator.EnrollManagedAccount(ctx, request)
	if err != nil {
		t.Fatal(err)
	}

	sessionKey, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := peer.IDFromPrivateKey(sessionKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := entity.RegisterSessionDirect(ctx, sessionID.String(), "fresh profile"); err != nil {
		t.Fatal(err)
	}
	return provider_spacewave.NewSessionClient(httpClient, env.cloudURL, "", sessionKey, sessionID.String()), enrolled.GetAccountId()
}

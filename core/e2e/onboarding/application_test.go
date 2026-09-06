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
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
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
}

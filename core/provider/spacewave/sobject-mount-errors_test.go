package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

func TestEnsureInitialStateReturnsTerminalErrorAfterRejectedPull(t *testing.T) {
	validatorPriv, _ := generateTestKeypair(t)
	_, otherPID := generateTestKeypair(t)
	state := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			ConsensusMode: sobject.SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR,
			Participants: []*sobject.SOParticipantConfig{{
				PeerId: otherPID.String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_VALIDATOR,
			}},
		},
		Root: buildTestSORoot(t, validatorPriv, 1, nil),
	}
	stateData := mustMarshalSOStateMessageSnapshotJSON(t, state)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sobject/so-invalid/state" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write(stateData)
	}))
	defer srv.Close()

	clientPriv, clientPID := generateTestKeypair(t)
	h := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, clientPriv, clientPID.String()),
		"so-invalid",
		"",
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
		clientPriv,
		clientPID,
		nil,
		nil,
		nil,
		nil,
	)

	err := h.ensureInitialState(context.Background(), SeedReasonColdSeed)
	if !errors.Is(err, errSharedObjectInitialStateRejected) {
		t.Fatalf("ensureInitialState() = %v, want terminal shared object mount error", err)
	}
}

func TestSobjectTrackerHoldTerminalMountError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tkr := &sobjectTracker{
		sobjectProm: promise.NewPromiseContainer[*SharedObject](),
		healthCtr:   ccontainer.NewCContainer[*sobject.SharedObjectHealth](nil),
	}
	wantErr := errors.Wrap(errSharedObjectInitialStateRejected, "terminal mount")

	done := make(chan error, 1)
	go func() {
		done <- tkr.holdTerminalMountError(ctx, wantErr)
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	_, err := tkr.sobjectProm.Await(waitCtx)
	if !errors.Is(err, errSharedObjectInitialStateRejected) {
		t.Fatalf("Await() = %v, want terminal shared object mount error", err)
	}
	health, err := tkr.healthCtr.WaitValue(waitCtx, nil)
	if err != nil {
		t.Fatalf("WaitValue() = %v", err)
	}
	if health.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED {
		t.Fatalf("expected closed health, got %v", health.GetStatus())
	}
	if health.GetCommonReason() != sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_INITIAL_STATE_REJECTED {
		t.Fatalf("expected initial-state-rejected reason, got %v", health.GetCommonReason())
	}
	if health.GetRemediationHint() != sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_CONTACT_OWNER {
		t.Fatalf("expected contact-owner hint, got %v", health.GetRemediationHint())
	}
	if health.GetError() != wantErr.Error() {
		t.Fatalf("expected detail %q, got %q", wantErr.Error(), health.GetError())
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("holdTerminalMountError() = %v, want context canceled", err)
	}
}

func TestIsTerminalSharedObjectMountErrorWrapped(t *testing.T) {
	err := errors.Wrap(errSharedObjectInitialStateRejected, "mount account settings")
	if !isTerminalSharedObjectMountError(err) {
		t.Fatalf("expected wrapped error to match terminal shared object mount error")
	}
}

func TestIsTerminalSharedObjectMountErrorCurrentKeyEpochMissing(t *testing.T) {
	err := errors.Wrap(errSharedObjectCurrentKeyEpochMissing, "rejoin")
	if !isTerminalSharedObjectMountError(err) {
		t.Fatalf("expected missing current key epoch to match terminal shared object mount error")
	}
}

func TestIsTerminalSharedObjectMountErrorNotParticipant(t *testing.T) {
	err := errors.Wrap(sobject.ErrNotParticipant, "sync config chain")
	if !isTerminalSharedObjectMountError(err) {
		t.Fatalf("expected not-participant error to match terminal shared object mount error")
	}
}

func TestIsTerminalSharedObjectMountErrorCloudAccessGated(t *testing.T) {
	cases := []string{
		"account_read_only",
		"dmca_blocked",
		"insufficient_role",
		"rbac_denied",
		"resource_not_found",
		"subscription_readonly",
		"subscription_required",
	}
	for _, code := range cases {
		err := errors.Wrap(&cloudError{
			StatusCode: http.StatusForbidden,
			Code:       code,
			Message:    "access gated",
		}, "mount account settings")
		if !isTerminalSharedObjectMountError(err) {
			t.Fatalf("expected %q to match terminal shared object mount error", code)
		}
	}
}

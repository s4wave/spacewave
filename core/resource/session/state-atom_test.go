package resource_session_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/core/session"
)

const (
	onboardingStateAtomStoreID = "session/setup/banner"
	dismissedOnboardingState   = `{"dismissed":true,"dismissedAt":123,"providerChoiceComplete":true,"backupComplete":false,"lockComplete":false}`
)

type waitSeqnoResult struct {
	seqno uint64
	err   error
}

// TestSessionStateAtomStoreSharedAcrossMounts verifies shared updates and remount persistence.
func TestSessionStateAtomStoreSharedAcrossMounts(t *testing.T) {
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	sessRef, _ := env.createSession(ctx, t)

	sessA, sessARef, err := session.ExMountSession(ctx, env.tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sessARef != nil {
			sessARef.Release()
		}
	}()

	sessB, sessBRef, err := session.ExMountSession(ctx, env.tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sessBRef != nil {
			sessBRef.Release()
		}
	}()

	storeA, err := sessA.AccessStateAtomStore(ctx, onboardingStateAtomStoreID)
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := sessB.AccessStateAtomStore(ctx, onboardingStateAtomStoreID)
	if err != nil {
		t.Fatal(err)
	}

	initialState, initialSeqno, err := storeB.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if initialState != "{}" {
		t.Fatalf("expected initial onboarding state '{}', got %q", initialState)
	}

	waitCh := make(chan waitSeqnoResult, 1)
	go func() {
		seqno, err := storeB.WaitSeqno(ctx, initialSeqno+1)
		waitCh <- waitSeqnoResult{seqno: seqno, err: err}
	}()

	nextSeqno, err := storeA.Set(ctx, dismissedOnboardingState)
	if err != nil {
		t.Fatal(err)
	}

	waitRes := <-waitCh
	if waitRes.err != nil {
		t.Fatal(waitRes.err)
	}
	if waitRes.seqno != nextSeqno {
		t.Fatalf("expected shared wait seqno %d, got %d", nextSeqno, waitRes.seqno)
	}

	sharedState, sharedSeqno, err := storeB.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sharedState != dismissedOnboardingState {
		t.Fatalf("expected shared onboarding state %q, got %q", dismissedOnboardingState, sharedState)
	}
	if sharedSeqno != nextSeqno {
		t.Fatalf("expected shared seqno %d, got %d", nextSeqno, sharedSeqno)
	}

	sessARef.Release()
	sessARef = nil
	sessBRef.Release()
	sessBRef = nil

	sessC, sessCRef, err := session.ExMountSession(ctx, env.tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessCRef.Release()

	storeC, err := sessC.AccessStateAtomStore(ctx, onboardingStateAtomStoreID)
	if err != nil {
		t.Fatal(err)
	}

	remountedState, _, err := storeC.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if remountedState != dismissedOnboardingState {
		t.Fatalf("expected remounted onboarding state %q, got %q", dismissedOnboardingState, remountedState)
	}
}

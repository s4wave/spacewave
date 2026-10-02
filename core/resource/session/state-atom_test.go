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
	// Start a test environment.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	// Create one local session.
	sessRef, _ := env.createSession(ctx, t)

	// Mount the session as the writer.
	sessA, sessARef, err := session.ExMountSession(ctx, env.tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sessARef != nil {
			sessARef.Release()
		}
	}()

	// Mount the same session as the reader.
	sessB, sessBRef, err := session.ExMountSession(ctx, env.tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sessBRef != nil {
			sessBRef.Release()
		}
	}()

	// Open the onboarding state-atom store on both mounts.
	storeA, err := sessA.AccessStateAtomStore(ctx, onboardingStateAtomStoreID)
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := sessB.AccessStateAtomStore(ctx, onboardingStateAtomStoreID)
	if err != nil {
		t.Fatal(err)
	}

	// Read the empty initial state from the second mount.
	initialState, initialSeqno, err := storeB.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if initialState != "{}" {
		t.Fatalf("expected initial onboarding state '{}', got %q", initialState)
	}

	// Wait for the next sequence on the second mount.
	waitCh := make(chan waitSeqnoResult, 1)
	go func() {
		seqno, err := storeB.WaitSeqno(ctx, initialSeqno+1)
		waitCh <- waitSeqnoResult{seqno: seqno, err: err}
	}()

	// Dismiss onboarding through the first mount.
	nextSeqno, err := storeA.Set(ctx, dismissedOnboardingState)
	if err != nil {
		t.Fatal(err)
	}

	// Accept the wait at the new sequence.
	waitRes := <-waitCh
	if waitRes.err != nil {
		t.Fatal(waitRes.err)
	}
	if waitRes.seqno != nextSeqno {
		t.Fatalf("expected shared wait seqno %d, got %d", nextSeqno, waitRes.seqno)
	}

	// Read the dismissed state from the second mount.
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

	// Release both mounts.
	sessARef.Release()
	sessARef = nil
	sessBRef.Release()
	sessBRef = nil

	// Mount the session again.
	sessC, sessCRef, err := session.ExMountSession(ctx, env.tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessCRef.Release()

	// Open the onboarding store on the new mount.
	storeC, err := sessC.AccessStateAtomStore(ctx, onboardingStateAtomStoreID)
	if err != nil {
		t.Fatal(err)
	}

	// Keep the dismissed state after remounting.
	remountedState, _, err := storeC.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if remountedState != dismissedOnboardingState {
		t.Fatalf("expected remounted onboarding state %q, got %q", dismissedOnboardingState, remountedState)
	}
}

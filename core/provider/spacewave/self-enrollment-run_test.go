//go:build !goscript

package provider_spacewave

import (
	"context"
	"testing"

	"github.com/aperturerobotics/util/routine"
)

func TestStartSelfEnrollmentRunSchedulesAccountOwnedRoutine(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	store := NewEntityKeyStore()
	store.Unlock(pid, priv)
	acc := &ProviderAccount{
		entityKeyStore: store,
	}
	acc.selfEnrollmentRun = newSelfEnrollmentRunState(acc)
	acc.selfEnrollmentRunRoutine = routine.NewStateRoutineContainer[*selfEnrollmentRunRequest](nil)
	acc.state.selfEnrollmentSummary = &SelfEnrollmentSummary{
		ids:           []string{"so-1"},
		generationKey: "gen-1",
		count:         1,
		loaded:        true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := acc.StartSelfEnrollmentRun(ctx); err != nil {
		t.Fatalf("StartSelfEnrollmentRun: %v", err)
	}
	cancel()

	state := acc.selfEnrollmentRunRoutine.GetState()
	if state == nil {
		t.Fatal("expected scheduled self-enrollment run state")
	}
	if state.generationKey != "gen-1" {
		t.Fatalf("generation key = %q, want gen-1", state.generationKey)
	}
	if len(state.ids) != 1 || state.ids[0] != "so-1" {
		t.Fatalf("scheduled ids = %#v, want [so-1]", state.ids)
	}
}

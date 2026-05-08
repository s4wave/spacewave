package provider_spacewave

import (
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
)

func TestBuildSelfEnrollmentProjectionPreservesBackendState(t *testing.T) {
	acc := &ProviderAccount{
		entityKeyStore: NewEntityKeyStore(),
	}
	acc.selfEnrollmentRun = newSelfEnrollmentRunState(acc)
	acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.state.selfEnrollmentSummary = &SelfEnrollmentSummary{
			ids:           []string{"so-2", "so-1"},
			generationKey: "gen-1",
			count:         2,
			loaded:        true,
		}
		acc.state.selfEnrollmentSkippedGenerationKey = "gen-1"
	})
	acc.selfEnrollmentRun.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.selfEnrollmentRun.running = true
		acc.selfEnrollmentRun.currentSharedObjectID = "so-2"
		acc.selfEnrollmentRun.completedIDs = []string{"so-1"}
		acc.selfEnrollmentRun.failures = []*SelfEnrollmentRunFailure{{
			SharedObjectID: "so-3",
			Err:            sobject.ErrNotParticipant,
		}}
	})

	got := acc.BuildSelfEnrollmentProjection()
	if got.GenerationKey != "gen-1" ||
		got.Count != 2 ||
		!got.PendingLoaded ||
		!got.CredentialRequired ||
		!got.Running ||
		got.CurrentSharedObjectID != "so-2" ||
		!got.Skipped ||
		got.SkippedGenerationKey != "gen-1" {
		t.Fatalf("unexpected projection: %+v", got)
	}
	if len(got.SharedObjectIDs) != 2 ||
		got.SharedObjectIDs[0] != "so-2" ||
		got.SharedObjectIDs[1] != "so-1" {
		t.Fatalf("shared object ids = %#v, want [so-2 so-1]", got.SharedObjectIDs)
	}
	if len(got.CompletedSharedObjectIDs) != 1 || got.CompletedSharedObjectIDs[0] != "so-1" {
		t.Fatalf("completed ids = %#v, want [so-1]", got.CompletedSharedObjectIDs)
	}
	if len(got.Failures) != 1 ||
		got.Failures[0].SharedObjectID != "so-3" ||
		got.Failures[0].Err != sobject.ErrNotParticipant {
		t.Fatalf("failures = %+v, want not-participant so-3 failure", got.Failures)
	}
}

func TestBuildSelfEnrollmentProjectionUsesEntityKeyAvailability(t *testing.T) {
	acc := &ProviderAccount{}
	acc.selfEnrollmentRun = newSelfEnrollmentRunState(acc)
	acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.state.selfEnrollmentSummary = &SelfEnrollmentSummary{
			ids:           []string{"so-1"},
			generationKey: "gen-1",
			count:         1,
			loaded:        true,
		}
	})
	if got := acc.BuildSelfEnrollmentProjection(); !got.CredentialRequired {
		t.Fatalf("credential required with no entity key store = false")
	}

	acc.entityKeyStore = NewEntityKeyStore()
	if got := acc.BuildSelfEnrollmentProjection(); !got.CredentialRequired {
		t.Fatalf("credential required with locked entity key store = false")
	}

	priv, pid := generateTestKeypair(t)
	acc.entityKeyStore.Unlock(pid, priv)
	if got := acc.BuildSelfEnrollmentProjection(); got.CredentialRequired {
		t.Fatalf("credential required with unlocked entity key store = true")
	}
}

func TestBuildSelfEnrollmentProjectionDoesNotRequireCredentialForEmptySummary(t *testing.T) {
	acc := &ProviderAccount{}
	acc.selfEnrollmentRun = newSelfEnrollmentRunState(acc)
	acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.state.selfEnrollmentSummary = &SelfEnrollmentSummary{loaded: true}
	})

	got := acc.BuildSelfEnrollmentProjection()
	if got.CredentialRequired {
		t.Fatalf("credential required for empty summary = true")
	}
	if !got.PendingLoaded {
		t.Fatalf("pending loaded = false, want true")
	}
}

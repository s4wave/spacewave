package provider_spacewave

import (
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
)

type selfEnrollmentProjectionCase struct {
	name               string
	ids                []string
	unlocked           bool
	running            bool
	skipped            bool
	failures           []*SelfEnrollmentRunFailure
	wantCount          uint32
	wantCredential     bool
	wantRunning        bool
	wantSkipped        bool
	wantFailureCount   int
	wantPendingLoaded  bool
	wantGenerationKey  string
	wantSkippedGenKey  string
	wantCurrentObject  string
	wantCompletedCount int
}

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

func TestBuildSelfEnrollmentProjectionStateMatrix(t *testing.T) {
	tests := []selfEnrollmentProjectionCase{
		{
			name:              "pending",
			ids:               []string{"so-1", "so-2"},
			unlocked:          true,
			wantCount:         2,
			wantPendingLoaded: true,
			wantGenerationKey: "gen-1",
		},
		{
			name:               "running",
			ids:                []string{"so-1", "so-2"},
			unlocked:           true,
			running:            true,
			wantCount:          2,
			wantRunning:        true,
			wantPendingLoaded:  true,
			wantGenerationKey:  "gen-1",
			wantCurrentObject:  "so-2",
			wantCompletedCount: 1,
		},
		{
			name:              "waiting-for-step-up",
			ids:               []string{"so-1", "so-2"},
			wantCount:         2,
			wantCredential:    true,
			wantPendingLoaded: true,
			wantGenerationKey: "gen-1",
		},
		{
			name:              "skipped",
			ids:               []string{"so-1", "so-2"},
			unlocked:          true,
			skipped:           true,
			wantCount:         2,
			wantSkipped:       true,
			wantPendingLoaded: true,
			wantGenerationKey: "gen-1",
			wantSkippedGenKey: "gen-1",
		},
		{
			name:     "failed",
			ids:      []string{"so-1", "so-2"},
			unlocked: true,
			failures: []*SelfEnrollmentRunFailure{{
				SharedObjectID: "so-2",
				Err:            sobject.ErrNotParticipant,
			}},
			wantCount:         2,
			wantFailureCount:  1,
			wantPendingLoaded: true,
			wantGenerationKey: "gen-1",
		},
		{
			name:              "ready",
			unlocked:          true,
			wantPendingLoaded: true,
			wantGenerationKey: "gen-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acc := newSelfEnrollmentProjectionStateMatrixAccount(t, tt)

			got := acc.BuildSelfEnrollmentProjection()
			if got.Count != tt.wantCount ||
				got.CredentialRequired != tt.wantCredential ||
				got.Running != tt.wantRunning ||
				got.Skipped != tt.wantSkipped ||
				len(got.Failures) != tt.wantFailureCount ||
				got.PendingLoaded != tt.wantPendingLoaded ||
				got.GenerationKey != tt.wantGenerationKey ||
				got.SkippedGenerationKey != tt.wantSkippedGenKey ||
				got.CurrentSharedObjectID != tt.wantCurrentObject ||
				len(got.CompletedSharedObjectIDs) != tt.wantCompletedCount {
				t.Fatalf("projection = %+v, want state case %+v", got, tt)
			}
		})
	}
}

func newSelfEnrollmentProjectionStateMatrixAccount(
	t *testing.T,
	tc selfEnrollmentProjectionCase,
) *ProviderAccount {
	t.Helper()

	acc := &ProviderAccount{}
	acc.selfEnrollmentRun = newSelfEnrollmentRunState(acc)
	acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.state.selfEnrollmentSummary = &SelfEnrollmentSummary{
			ids:           tc.ids,
			generationKey: "gen-1",
			count:         uint32(len(tc.ids)),
			loaded:        true,
		}
		if tc.skipped {
			acc.state.selfEnrollmentSkippedGenerationKey = "gen-1"
		}
	})
	if tc.unlocked {
		acc.entityKeyStore = NewEntityKeyStore()
		priv, pid := generateTestKeypair(t)
		acc.entityKeyStore.Unlock(pid, priv)
	}
	acc.selfEnrollmentRun.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.selfEnrollmentRun.running = tc.running
		if tc.running {
			acc.selfEnrollmentRun.currentSharedObjectID = "so-2"
			acc.selfEnrollmentRun.completedIDs = []string{"so-1"}
		}
		acc.selfEnrollmentRun.failures = tc.failures
	})
	return acc
}

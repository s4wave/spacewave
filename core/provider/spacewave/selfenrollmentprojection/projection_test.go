package selfenrollmentprojection

import (
	"errors"
	"testing"
)

var errNotParticipant = errors.New("not participant")

type projectionCase struct {
	name               string
	ids                []string
	unlocked           bool
	running            bool
	skipped            bool
	failures           []*RunFailure
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

func TestBuildPreservesBackendState(t *testing.T) {
	got := Build(Snapshot{
		Summary: Summary{
			Present:       true,
			SharedObjects: []string{"so-2", "so-1"},
			GenerationKey: "gen-1",
			Count:         2,
			Loaded:        true,
		},
		Run: &RunSnapshot{
			Running:               true,
			CurrentSharedObjectID: "so-2",
			CompletedIDs:          []string{"so-1"},
			Failures: []*RunFailure{{
				SharedObjectID: "so-3",
				Err:            errNotParticipant,
			}},
		},
		SkippedGenerationKey: "gen-1",
		HasCredentialStore:   true,
	})
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
		got.Failures[0].Err != errNotParticipant {
		t.Fatalf("failures = %+v, want not-participant so-3 failure", got.Failures)
	}
}

func TestBuildUsesCredentialAvailability(t *testing.T) {
	summary := Summary{
		Present:       true,
		SharedObjects: []string{"so-1"},
		GenerationKey: "gen-1",
		Count:         1,
		Loaded:        true,
	}
	if got := Build(Snapshot{Summary: summary}); !got.CredentialRequired {
		t.Fatalf("credential required with no credential store = false")
	}
	if got := Build(Snapshot{
		Summary:            summary,
		HasCredentialStore: true,
	}); !got.CredentialRequired {
		t.Fatalf("credential required with locked credential store = false")
	}
	if got := Build(Snapshot{
		Summary:                 summary,
		HasCredentialStore:      true,
		UnlockedCredentialCount: 1,
	}); got.CredentialRequired {
		t.Fatalf("credential required with unlocked credential store = true")
	}
}

func TestBuildDoesNotRequireCredentialForEmptySummary(t *testing.T) {
	got := Build(Snapshot{Summary: Summary{
		Present: true,
		Loaded:  true,
	}})
	if got.CredentialRequired {
		t.Fatalf("credential required for empty summary = true")
	}
	if !got.PendingLoaded {
		t.Fatalf("pending loaded = false, want true")
	}
}

func TestBuildStateMatrix(t *testing.T) {
	tests := []projectionCase{
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
			failures: []*RunFailure{{
				SharedObjectID: "so-2",
				Err:            errNotParticipant,
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
			got := Build(newStateMatrixSnapshot(tt))
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

func TestBuildSnapshotsSlices(t *testing.T) {
	summaryIDs := []string{"so-1"}
	completedIDs := []string{"so-2"}
	failures := []*RunFailure{{SharedObjectID: "so-3", Err: errNotParticipant}}
	got := Build(Snapshot{
		Summary: Summary{
			Present:       true,
			SharedObjects: summaryIDs,
			GenerationKey: "gen-1",
			Count:         1,
			Loaded:        true,
		},
		Run: &RunSnapshot{
			CompletedIDs: completedIDs,
			Failures:     failures,
		},
		HasCredentialStore:      true,
		UnlockedCredentialCount: 1,
	})

	summaryIDs[0] = "changed-summary"
	completedIDs[0] = "changed-completed"
	failures[0].SharedObjectID = "changed-failure"
	if got.SharedObjectIDs[0] != "so-1" ||
		got.CompletedSharedObjectIDs[0] != "so-2" ||
		got.Failures[0].SharedObjectID != "so-3" {
		t.Fatalf("projection retained caller-owned slices: %+v", got)
	}
}

func newStateMatrixSnapshot(tc projectionCase) Snapshot {
	snapshot := Snapshot{
		Summary: Summary{
			Present:       true,
			SharedObjects: tc.ids,
			GenerationKey: "gen-1",
			Count:         uint32(len(tc.ids)),
			Loaded:        true,
		},
		Run: &RunSnapshot{
			Running:  tc.running,
			Failures: tc.failures,
		},
		HasCredentialStore: tc.unlocked,
	}
	if tc.unlocked {
		snapshot.UnlockedCredentialCount = 1
	}
	if tc.skipped {
		snapshot.SkippedGenerationKey = "gen-1"
	}
	if tc.running {
		snapshot.Run.CurrentSharedObjectID = "so-2"
		snapshot.Run.CompletedIDs = []string{"so-1"}
	}
	return snapshot
}

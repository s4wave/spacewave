package selfenrollmentprojection

import "slices"

// Projection is the backend state projection for Session Self-Enrollment.
type Projection struct {
	SharedObjectIDs          []string
	GenerationKey            string
	Count                    uint32
	PendingLoaded            bool
	CredentialRequired       bool
	Running                  bool
	CurrentSharedObjectID    string
	CompletedSharedObjectIDs []string
	Skipped                  bool
	SkippedGenerationKey     string
	Failures                 []*RunFailure
}

// Summary carries the cached post-login self-enrollment predicate.
type Summary struct {
	Present       bool
	SharedObjects []string
	GenerationKey string
	Count         uint32
	Loaded        bool
}

// RunFailure describes a failed per-object enrollment.
type RunFailure struct {
	SharedObjectID string
	Err            error
}

// RunSnapshot is a snapshot of the visible self-enrollment run.
type RunSnapshot struct {
	Running               bool
	CurrentSharedObjectID string
	CompletedIDs          []string
	Failures              []*RunFailure
}

// Snapshot carries Provider Account state needed by the self-enrollment
// projection.
type Snapshot struct {
	Summary                 Summary
	Run                     *RunSnapshot
	SkippedGenerationKey    string
	HasCredentialStore      bool
	UnlockedCredentialCount int
}

// Build builds a Session Self-Enrollment projection from cached backend state.
func Build(snapshot Snapshot) *Projection {
	proj := &Projection{
		SkippedGenerationKey: snapshot.SkippedGenerationKey,
	}
	if snapshot.Run != nil {
		proj.Running = snapshot.Run.Running
		proj.CurrentSharedObjectID = snapshot.Run.CurrentSharedObjectID
		proj.CompletedSharedObjectIDs = slices.Clone(snapshot.Run.CompletedIDs)
		proj.Failures = CloneRunFailures(snapshot.Run.Failures)
	}
	if !snapshot.Summary.Present {
		return proj
	}
	proj.SharedObjectIDs = slices.Clone(snapshot.Summary.SharedObjects)
	proj.GenerationKey = snapshot.Summary.GenerationKey
	proj.Count = snapshot.Summary.Count
	proj.PendingLoaded = snapshot.Summary.Loaded
	proj.CredentialRequired = snapshot.Summary.Count != 0 &&
		(!snapshot.HasCredentialStore || snapshot.UnlockedCredentialCount == 0)
	proj.Skipped = snapshot.SkippedGenerationKey != "" &&
		snapshot.SkippedGenerationKey == snapshot.Summary.GenerationKey
	return proj
}

// CloneRunFailures snapshots run failures.
func CloneRunFailures(failures []*RunFailure) []*RunFailure {
	if len(failures) == 0 {
		return nil
	}
	next := make([]*RunFailure, len(failures))
	for i, failure := range failures {
		if failure != nil {
			next[i] = &RunFailure{
				SharedObjectID: failure.SharedObjectID,
				Err:            failure.Err,
			}
		}
	}
	return next
}

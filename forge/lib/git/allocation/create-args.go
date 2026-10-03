package forge_lib_git_allocation

import timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"

// CreateArgs contains inputs for creating or reusing an allocation.
type CreateArgs struct {
	// ObjectKey overrides the deterministic allocation identity.
	ObjectKey string
	// JobObjectKey retains the checkout for one Job.
	JobObjectKey string
	// PeerID authorizes Workdir writes for a Job allocation.
	PeerID string
	// ExecutionObjectKey selects the Execution owner, exclusive with JobObjectKey.
	ExecutionObjectKey string
	// PassObjectKey identifies the Pass containing the Execution.
	PassObjectKey string
	// RepoObjectKey identifies the shared Git object database.
	RepoObjectKey string
	// WorktreeObjectKey identifies the allocated checkout.
	WorktreeObjectKey string
	// WorkdirRequired requires a mutable Workdir identity.
	WorkdirRequired bool
	// WorkdirObjectKey identifies its mutable filesystem.
	WorkdirObjectKey string
	// BaseCommitHash pins the admitted base commit.
	BaseCommitHash string
	// BranchRef selects the branch, or is empty for a detached Job checkout.
	BranchRef string
	// PathFamily identifies the visible path family.
	PathFamily string
	// EvidenceObjectKey references allocation evidence.
	EvidenceObjectKey string
	// Status sets the allocation status, defaulting to allocated.
	Status string
	// CollisionState sets collision handling, defaulting to none.
	CollisionState string
	// StaleBaseState sets the base observation, defaulting to unchecked.
	StaleBaseState string
	// CleanupState sets cleanup state, defaulting to active.
	CleanupState string
	// Timestamp records creation time, defaulting to now.
	Timestamp *timestamp.Timestamp
}

package forge_lib_git_allocation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/fastjson"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	"github.com/s4wave/spacewave/net/peer"
)

const (
	// AllocationTypeID is the world object type identifier for a Git Worktree allocation.
	AllocationTypeID = "forge/lib/git/worktree-allocation"

	allocationGraphPathLimit uint32 = 1_000_000
)

var (
	// PredExecutionToAllocation links a Forge Execution to a Git Worktree allocation.
	PredExecutionToAllocation = quad.IRI("forge/git/execution-allocation")
	// PredJobToAllocation links a Forge Job to its retained Git allocation.
	PredJobToAllocation = quad.IRI("forge/git/job-allocation")
	// PredPassToAllocation links a Forge Pass to a Git Worktree allocation.
	PredPassToAllocation = quad.IRI("forge/git/pass-allocation")
	// PredAllocationToRepo links an allocation to the owning Git Repo object.
	PredAllocationToRepo = quad.IRI("forge/git/allocation-repo")
	// PredAllocationToWorktree links an allocation to the allocated Git Worktree object.
	PredAllocationToWorktree = quad.IRI("forge/git/allocation-worktree")
)

// Allocation records provenance for one Forge-owned Git Worktree allocation.
type Allocation struct {
	// JobObjectKey is the Job retaining this checkout across its Executions.
	JobObjectKey string `json:"jobObjectKey,omitempty"`
	// PeerID is the external World identity authorized to operate a Job checkout.
	PeerID string `json:"peerId,omitempty"`
	// ExecutionObjectKey is the owning Forge Execution object key.
	ExecutionObjectKey string `json:"executionObjectKey,omitempty"`
	// PassObjectKey is the optional owning Forge Pass object key.
	PassObjectKey string `json:"passObjectKey,omitempty"`
	// RepoObjectKey is the Git Repo object key.
	RepoObjectKey string `json:"repoObjectKey,omitempty"`
	// WorktreeObjectKey is the allocated Git Worktree object key.
	WorktreeObjectKey string `json:"worktreeObjectKey,omitempty"`
	// WorkdirObjectKey is the mutable Workdir UnixFS object linked to the Worktree.
	WorkdirObjectKey string `json:"workdirObjectKey,omitempty"`
	// BaseCommitHash is the pinned base commit used for branch allocation.
	BaseCommitHash string `json:"baseCommitHash,omitempty"`
	// BranchRef is the branch/ref assigned to the allocation.
	BranchRef string `json:"branchRef,omitempty"`
	// PathFamily is the normalized visible path family that triggered allocation.
	PathFamily string `json:"pathFamily,omitempty"`
	// EvidenceObjectKey points at raw allocation Evidence when present.
	EvidenceObjectKey string `json:"evidenceObjectKey,omitempty"`
	// Status is the allocator lifecycle state.
	Status string `json:"status,omitempty"`
	// CollisionState records branch/key collision handling state.
	CollisionState string `json:"collisionState,omitempty"`
	// StaleBaseState records whether the pinned base is current for allocation.
	StaleBaseState string `json:"staleBaseState,omitempty"`
	// CleanupState records cleanup lifecycle for the allocated Worktree.
	CleanupState string `json:"cleanupState,omitempty"`
	// Cleanup records the terminal runtime cleanup receipt, set by RecordCleanup.
	Cleanup *forge_runtime.CleanupReceipt `json:"cleanup,omitempty"`
	// Timestamp is the allocation timestamp.
	Timestamp *timestamp.Timestamp `json:"timestamp,omitempty"`
}

// BuildObjectKey builds a deterministic allocation object key.
func BuildObjectKey(ownerObjectKey, repoObjectKey, baseCommitHash, pathFamily string) string {
	hash := sha256.Sum256([]byte(strings.Join([]string{
		ownerObjectKey,
		repoObjectKey,
		baseCommitHash,
		pathFamily,
	}, "\x00")))
	return "forge/git/allocation/" + hex.EncodeToString(hash[:12])
}

// NewAllocationBlock constructs an empty Allocation block.
func NewAllocationBlock() block.Block {
	return &Allocation{}
}

// CreateOrReuse creates an allocation object or returns the existing matching allocation.
func CreateOrReuse(
	ctx context.Context,
	ws world.WorldState,
	args CreateArgs,
) (*Allocation, string, *bucket.ObjectRef, error) {
	// Derive the allocation identity from the retained Job or owning Execution.
	ownerKey := args.JobObjectKey
	if ownerKey == "" {
		ownerKey = args.ExecutionObjectKey
	}
	if args.ObjectKey == "" {
		args.ObjectKey = BuildObjectKey(
			ownerKey,
			args.RepoObjectKey,
			args.BaseCommitHash,
			args.PathFamily,
		)
	}

	// Freeze provenance and default its initial lifecycle state.
	alloc := &Allocation{
		JobObjectKey:       args.JobObjectKey,
		PeerID:             args.PeerID,
		ExecutionObjectKey: args.ExecutionObjectKey,
		PassObjectKey:      args.PassObjectKey,
		RepoObjectKey:      args.RepoObjectKey,
		WorktreeObjectKey:  args.WorktreeObjectKey,
		WorkdirObjectKey:   args.WorkdirObjectKey,
		BaseCommitHash:     args.BaseCommitHash,
		BranchRef:          args.BranchRef,
		PathFamily:         args.PathFamily,
		EvidenceObjectKey:  args.EvidenceObjectKey,
		Status:             args.Status,
		CollisionState:     args.CollisionState,
		StaleBaseState:     args.StaleBaseState,
		CleanupState:       args.CleanupState,
		Timestamp:          args.Timestamp,
	}
	if alloc.Status == "" {
		alloc.Status = "allocated"
	}
	if alloc.CollisionState == "" {
		alloc.CollisionState = "none"
	}
	if alloc.StaleBaseState == "" {
		alloc.StaleBaseState = "unchecked"
	}
	if alloc.CleanupState == "" {
		alloc.CleanupState = "active"
	}
	if alloc.Timestamp == nil {
		alloc.Timestamp = timestamp.Now()
	}
	if args.WorkdirRequired && args.WorkdirObjectKey == "" {
		return nil, "", nil, errors.New("sandbox allocation requires a workdir object key")
	}
	if err := alloc.Validate(); err != nil {
		return nil, "", nil, err
	}

	// Reuse only the same admitted checkout and allocation authority.
	existing, found, err := ws.GetObject(ctx, args.ObjectKey)
	defer world.ReleaseObjectState(existing)
	if err != nil {
		return nil, "", nil, err
	}
	if found {
		existingAlloc, err := Lookup(ctx, ws, args.ObjectKey)
		if err != nil {
			return nil, "", nil, err
		}
		if args.WorkdirRequired && existingAlloc.GetWorkdirObjectKey() == "" {
			return nil, "", nil, errors.Errorf(
				"existing allocation %s lacks the required workdir identity; clean break required",
				args.ObjectKey,
			)
		}
		if !existingAlloc.sameAllocation(alloc) {
			return nil, "", nil, errors.Errorf("allocation key collision: %s", args.ObjectKey)
		}
		ref, _, err := existing.GetRootRef(ctx)
		return existingAlloc, args.ObjectKey, ref, err
	}

	// Publish the allocation body and its graph relationships together.
	createdObject, rootRef, err := world.CreateWorldObject(ctx, ws, args.ObjectKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(alloc, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return nil, "", nil, err
	}
	if err := setAllocationQuads(ctx, ws, args.ObjectKey, alloc); err != nil {
		return nil, "", nil, err
	}
	return alloc, args.ObjectKey, rootRef, nil
}

// Lookup loads an Allocation by object key.
func Lookup(ctx context.Context, ws world.WorldState, objKey string) (*Allocation, error) {
	return world.LookupObjectBody[*Allocation](ctx, ws, objKey, NewAllocationBlock)
}

// ListExecutionAllocations lists allocation object keys linked to Executions.
func ListExecutionAllocations(ctx context.Context, ws world.WorldState, executionKeys ...string) ([]string, error) {
	return world.CollectGraphPathStepWithKeys(
		ctx,
		ws,
		executionKeys,
		world.GraphPathDirectionOut,
		PredExecutionToAllocation.String(),
		allocationGraphPathLimit,
	)
}

// ListPassAllocations lists allocation object keys linked to Passes.
func ListPassAllocations(ctx context.Context, ws world.WorldState, passKeys ...string) ([]string, error) {
	return world.CollectGraphPathStepWithKeys(
		ctx,
		ws,
		passKeys,
		world.GraphPathDirectionOut,
		PredPassToAllocation.String(),
		allocationGraphPathLimit,
	)
}

// ListAllocationRepos lists Repo object keys linked to allocations.
func ListAllocationRepos(ctx context.Context, ws world.WorldState, allocationKeys ...string) ([]string, error) {
	return world.CollectGraphPathStepWithKeys(
		ctx,
		ws,
		allocationKeys,
		world.GraphPathDirectionOut,
		PredAllocationToRepo.String(),
		allocationGraphPathLimit,
	)
}

// ListAllocationWorktrees lists Worktree object keys linked to allocations.
func ListAllocationWorktrees(ctx context.Context, ws world.WorldState, allocationKeys ...string) ([]string, error) {
	return world.CollectGraphPathStepWithKeys(
		ctx,
		ws,
		allocationKeys,
		world.GraphPathDirectionOut,
		PredAllocationToWorktree.String(),
		allocationGraphPathLimit,
	)
}

// Validate validates an Allocation.
func (a *Allocation) Validate() error {
	// Require one owner and complete checkout provenance.
	switch {
	case (a.GetExecutionObjectKey() == "") == (a.GetJobObjectKey() == ""):
		return errors.New("allocation requires exactly one Job or Execution owner")
	case a.GetJobObjectKey() != "" && a.GetPassObjectKey() != "":
		return errors.New("Job allocation cannot have a Pass owner")
	case a.GetRepoObjectKey() == "":
		return errors.New("repo_object_key cannot be empty")
	case a.GetWorktreeObjectKey() == "":
		return errors.New("worktree_object_key cannot be empty")
	case a.GetBaseCommitHash() == "":
		return errors.New("base_commit_hash cannot be empty")
	case a.GetJobObjectKey() == "" && a.GetBranchRef() == "":
		return errors.New("branch_ref cannot be empty")
	case a.GetPathFamily() == "":
		return errors.New("path_family cannot be empty")
	case a.GetStatus() == "":
		return errors.New("status cannot be empty")
	case a.GetCollisionState() == "":
		return errors.New("collision_state cannot be empty")
	case a.GetStaleBaseState() == "":
		return errors.New("stale_base_state cannot be empty")
	case a.GetCleanupState() == "":
		return errors.New("cleanup_state cannot be empty")
	}

	// Validate the Job grant and any terminal runtime receipt.
	if a.GetJobObjectKey() != "" {
		if _, err := peer.IDB58Decode(a.PeerID); err != nil {
			return errors.Wrap(err, "Job allocation peer ID")
		}
		if a.GetWorkdirObjectKey() == "" {
			return errors.New("Job allocation requires a Workdir")
		}
	}
	if cleanup := a.GetCleanup(); cleanup != nil {
		if err := cleanup.Validate(); err != nil {
			return errors.Wrap(err, "cleanup")
		}
	}
	if err := a.GetTimestamp().Validate(false); err != nil {
		return errors.Wrap(err, "timestamp")
	}
	return nil
}

// GetJobObjectKey returns the Job retaining the checkout.
func (a *Allocation) GetJobObjectKey() string {
	if a != nil {
		return a.JobObjectKey
	}
	return ""
}

// GetExecutionObjectKey returns the owning Execution object key.
func (a *Allocation) GetExecutionObjectKey() string {
	if a != nil {
		return a.ExecutionObjectKey
	}
	return ""
}

// GetPassObjectKey returns the optional owning Pass object key.
func (a *Allocation) GetPassObjectKey() string {
	if a != nil {
		return a.PassObjectKey
	}
	return ""
}

// GetRepoObjectKey returns the Repo object key.
func (a *Allocation) GetRepoObjectKey() string {
	if a != nil {
		return a.RepoObjectKey
	}
	return ""
}

// GetWorktreeObjectKey returns the Worktree object key.
func (a *Allocation) GetWorktreeObjectKey() string {
	if a != nil {
		return a.WorktreeObjectKey
	}
	return ""
}

// GetWorkdirObjectKey returns the mutable Workdir object key.
func (a *Allocation) GetWorkdirObjectKey() string {
	if a != nil {
		return a.WorkdirObjectKey
	}
	return ""
}

// GetCleanup returns the terminal runtime cleanup receipt.
func (a *Allocation) GetCleanup() *forge_runtime.CleanupReceipt {
	if a != nil {
		return a.Cleanup
	}
	return nil
}

// GetBaseCommitHash returns the base commit hash.
func (a *Allocation) GetBaseCommitHash() string {
	if a != nil {
		return a.BaseCommitHash
	}
	return ""
}

// GetBranchRef returns the branch/ref.
func (a *Allocation) GetBranchRef() string {
	if a != nil {
		return a.BranchRef
	}
	return ""
}

// GetPathFamily returns the visible path family.
func (a *Allocation) GetPathFamily() string {
	if a != nil {
		return a.PathFamily
	}
	return ""
}

// GetEvidenceObjectKey returns the raw Evidence object key.
func (a *Allocation) GetEvidenceObjectKey() string {
	if a != nil {
		return a.EvidenceObjectKey
	}
	return ""
}

// GetStatus returns the allocation status.
func (a *Allocation) GetStatus() string {
	if a != nil {
		return a.Status
	}
	return ""
}

// GetCollisionState returns the branch/key collision state.
func (a *Allocation) GetCollisionState() string {
	if a != nil {
		return a.CollisionState
	}
	return ""
}

// GetStaleBaseState returns the stale-base state.
func (a *Allocation) GetStaleBaseState() string {
	if a != nil {
		return a.StaleBaseState
	}
	return ""
}

// GetCleanupState returns the cleanup state.
func (a *Allocation) GetCleanupState() string {
	if a != nil {
		return a.CleanupState
	}
	return ""
}

// GetTimestamp returns the allocation timestamp.
func (a *Allocation) GetTimestamp() *timestamp.Timestamp {
	if a != nil {
		return a.Timestamp
	}
	return nil
}

// Reset resets the block.
func (a *Allocation) Reset() {
	*a = Allocation{}
}

// MarshalBlock marshals the block to binary.
func (a *Allocation) MarshalBlock() ([]byte, error) {
	return a.MarshalJSON()
}

// UnmarshalBlock unmarshals the block from binary.
func (a *Allocation) UnmarshalBlock(data []byte) error {
	return a.UnmarshalJSON(data)
}

// MarshalJSON marshals the Allocation to JSON without reflection.
func (a *Allocation) MarshalJSON() ([]byte, error) {
	// Preserve nil block encoding.
	if a == nil {
		return []byte("null"), nil
	}

	// Encode the allocation owner and checkout identities.
	var arena fastjson.Arena
	obj := arena.NewObject()
	setStringJSONField(&arena, obj, "jobObjectKey", a.JobObjectKey)
	setStringJSONField(&arena, obj, "peerId", a.PeerID)
	setStringJSONField(&arena, obj, "executionObjectKey", a.ExecutionObjectKey)
	setStringJSONField(&arena, obj, "passObjectKey", a.PassObjectKey)
	setStringJSONField(&arena, obj, "repoObjectKey", a.RepoObjectKey)
	setStringJSONField(&arena, obj, "worktreeObjectKey", a.WorktreeObjectKey)

	// Encode the immutable base and visible path provenance.
	setStringJSONField(&arena, obj, "workdirObjectKey", a.WorkdirObjectKey)
	setStringJSONField(&arena, obj, "baseCommitHash", a.BaseCommitHash)
	setStringJSONField(&arena, obj, "branchRef", a.BranchRef)
	setStringJSONField(&arena, obj, "pathFamily", a.PathFamily)
	setStringJSONField(&arena, obj, "evidenceObjectKey", a.EvidenceObjectKey)

	// Encode lifecycle state independently of the retained checkout identity.
	setStringJSONField(&arena, obj, "status", a.Status)
	setStringJSONField(&arena, obj, "collisionState", a.CollisionState)
	setStringJSONField(&arena, obj, "staleBaseState", a.StaleBaseState)
	setStringJSONField(&arena, obj, "cleanupState", a.CleanupState)

	// Include the latest runtime cleanup receipt when present.
	if cleanup := a.GetCleanup(); cleanup != nil {
		cleanupJSON, err := cleanup.MarshalJSON()
		if err != nil {
			return nil, errors.Wrap(err, "marshal cleanup")
		}
		cleanupValue, err := parseJSONValue(&arena, cleanupJSON)
		if err != nil {
			return nil, errors.Wrap(err, "marshal cleanup")
		}
		obj.Set("cleanup", cleanupValue)
	}

	// Include the allocation creation timestamp.
	if a.Timestamp != nil {
		timestampJSON, err := a.Timestamp.MarshalJSON()
		if err != nil {
			return nil, errors.Wrap(err, "marshal timestamp")
		}
		timestampValue, err := parseJSONValue(&arena, timestampJSON)
		if err != nil {
			return nil, errors.Wrap(err, "parse timestamp")
		}
		obj.Set("timestamp", timestampValue)
	}
	return obj.MarshalTo(nil), nil
}

// setStringJSONField omits empty allocation JSON strings.
func setStringJSONField(arena *fastjson.Arena, obj *fastjson.Value, key, value string) {
	if value != "" {
		obj.Set(key, arena.NewString(value))
	}
}

// parseJSONValue copies a separately encoded message into the allocation arena.
func parseJSONValue(arena *fastjson.Arena, data []byte) (*fastjson.Value, error) {
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return nil, err
	}
	return arena.DeepCopyValue(value), nil
}

// UnmarshalJSON unmarshals the Allocation from JSON without reflection.
func (a *Allocation) UnmarshalJSON(data []byte) error {
	// Parse the allocation block and distinguish null from an object.
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return err
	}
	if value.Type() == fastjson.TypeNull {
		*a = Allocation{}
		return nil
	}
	if value.Type() != fastjson.TypeObject {
		return errors.New("allocation must be object")
	}

	// Restore the allocation owner and checkout identities.
	a.JobObjectKey = string(value.GetStringBytes("jobObjectKey"))
	a.PeerID = string(value.GetStringBytes("peerId"))
	a.ExecutionObjectKey = string(value.GetStringBytes("executionObjectKey"))
	a.PassObjectKey = string(value.GetStringBytes("passObjectKey"))
	a.RepoObjectKey = string(value.GetStringBytes("repoObjectKey"))
	a.WorktreeObjectKey = string(value.GetStringBytes("worktreeObjectKey"))

	// Restore the admitted base and path provenance.
	a.WorkdirObjectKey = string(value.GetStringBytes("workdirObjectKey"))
	a.BaseCommitHash = string(value.GetStringBytes("baseCommitHash"))
	a.BranchRef = string(value.GetStringBytes("branchRef"))
	a.PathFamily = string(value.GetStringBytes("pathFamily"))
	a.EvidenceObjectKey = string(value.GetStringBytes("evidenceObjectKey"))

	// Restore lifecycle state separately from checkout provenance.
	a.Status = string(value.GetStringBytes("status"))
	a.CollisionState = string(value.GetStringBytes("collisionState"))
	a.StaleBaseState = string(value.GetStringBytes("staleBaseState"))
	a.CleanupState = string(value.GetStringBytes("cleanupState"))

	// Decode the optional runtime receipt, clearing any earlier value.
	a.Cleanup = nil
	if cleanupValue := value.Get("cleanup"); cleanupValue != nil && cleanupValue.Type() == fastjson.TypeObject {
		cleanup := &forge_runtime.CleanupReceipt{}
		if err := cleanup.UnmarshalJSON(cleanupValue.MarshalTo(nil)); err != nil {
			return errors.Wrap(err, "unmarshal cleanup")
		}
		a.Cleanup = cleanup
	}

	// Decode the creation timestamp, clearing any earlier value.
	a.Timestamp = nil
	if timestampValue := value.Get("timestamp"); timestampValue != nil && timestampValue.Type() != fastjson.TypeNull {
		ts := &timestamp.Timestamp{}
		if err := ts.UnmarshalJSON(timestampValue.MarshalTo(nil)); err != nil {
			return errors.Wrap(err, "unmarshal timestamp")
		}
		a.Timestamp = ts
	}
	return nil
}

// sameAllocation compares immutable custody and admitted checkout provenance.
func (a *Allocation) sameAllocation(other *Allocation) bool {
	return a.GetJobObjectKey() == other.GetJobObjectKey() &&
		a.PeerID == other.PeerID &&
		a.GetExecutionObjectKey() == other.GetExecutionObjectKey() &&
		a.GetPassObjectKey() == other.GetPassObjectKey() &&
		a.GetRepoObjectKey() == other.GetRepoObjectKey() &&
		a.GetWorktreeObjectKey() == other.GetWorktreeObjectKey() &&
		a.GetWorkdirObjectKey() == other.GetWorkdirObjectKey() &&
		a.GetBaseCommitHash() == other.GetBaseCommitHash() &&
		a.GetBranchRef() == other.GetBranchRef() &&
		a.GetPathFamily() == other.GetPathFamily() &&
		a.GetCollisionState() == other.GetCollisionState() &&
		a.GetStaleBaseState() == other.GetStaleBaseState() &&
		a.GetCleanupState() == other.GetCleanupState()
}

// setAllocationQuads publishes ownership and checkout references.
func setAllocationQuads(ctx context.Context, ws world.WorldState, objKey string, alloc *Allocation) error {
	// Link the allocation to its Job or Execution owner.
	ownerKey, predicate := alloc.GetJobObjectKey(), PredJobToAllocation
	if ownerKey == "" {
		ownerKey, predicate = alloc.GetExecutionObjectKey(), PredExecutionToAllocation
	}
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(ownerKey, predicate.String(), objKey, "")); err != nil {
		return err
	}

	// Preserve the Execution owner's Pass relationship when supplied.
	if passKey := alloc.GetPassObjectKey(); passKey != "" {
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(passKey, PredPassToAllocation.String(), objKey, "")); err != nil {
			return err
		}
	}

	// Link the shared repository and private checkout.
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(objKey, PredAllocationToRepo.String(), alloc.GetRepoObjectKey(), "")); err != nil {
		return err
	}
	return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(objKey, PredAllocationToWorktree.String(), alloc.GetWorktreeObjectKey(), ""))
}

// UnmarshalAllocation unmarshals an Allocation from a block cursor.
func UnmarshalAllocation(ctx context.Context, bcs *block.Cursor) (*Allocation, error) {
	return block.UnmarshalBlock[*Allocation](ctx, bcs, NewAllocationBlock)
}

// RecordCleanup records the terminal runtime cleanup receipt on one
// allocation. Runtime release stays separate from retention: the Worktree and
// committed Workdir bytes remain readable after the receipt lands.
func RecordCleanup(
	ctx context.Context,
	ws world.WorldState,
	allocationObjectKey string,
	receipt *forge_runtime.CleanupReceipt,
) (*Allocation, *bucket.ObjectRef, error) {
	// Validate the allocation key and runtime receipt before mutation.
	if allocationObjectKey == "" {
		return nil, nil, errors.New("allocation_object_key cannot be empty")
	}
	if err := receipt.Validate(); err != nil {
		return nil, nil, errors.Wrap(err, "cleanup receipt")
	}

	// Record runtime cleanup without ending a retained Job workspace.
	var out *Allocation
	ref, _, err := world.AccessWorldObject(ctx, ws, allocationObjectKey, true, func(bcs *block.Cursor) error {
		// Decode the allocation before applying its runtime receipt.
		alloc, err := UnmarshalAllocation(ctx, bcs)
		if err != nil {
			return err
		}

		// Execution cleanup never releases a checkout retained by a Job.
		alloc.Cleanup = receipt
		if alloc.GetJobObjectKey() == "" {
			alloc.CleanupState = "cleanup-partial"
			if receipt.Complete() {
				alloc.CleanupState = "released"
			}
		}
		if err := alloc.Validate(); err != nil {
			return err
		}

		// Publish the validated receipt and return its updated root.
		bcs.SetBlock(alloc, true)
		out = alloc
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return out, ref, nil
}

// _ is a type assertion.
var _ block.Block = (*Allocation)(nil)

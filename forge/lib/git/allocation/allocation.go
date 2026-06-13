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
)

const (
	// AllocationTypeID is the world object type identifier for a Git Worktree allocation.
	AllocationTypeID = "forge/lib/git/worktree-allocation"

	allocationGraphPathLimit uint32 = 1_000_000
)

var (
	// PredExecutionToAllocation links a Forge Execution to a Git Worktree allocation.
	PredExecutionToAllocation = quad.IRI("forge/git/execution-allocation")
	// PredPassToAllocation links a Forge Pass to a Git Worktree allocation.
	PredPassToAllocation = quad.IRI("forge/git/pass-allocation")
	// PredAllocationToRepo links an allocation to the owning Git Repo object.
	PredAllocationToRepo = quad.IRI("forge/git/allocation-repo")
	// PredAllocationToWorktree links an allocation to the allocated Git Worktree object.
	PredAllocationToWorktree = quad.IRI("forge/git/allocation-worktree")
)

// Allocation records provenance for one Forge-owned Git Worktree allocation.
type Allocation struct {
	// ExecutionObjectKey is the owning Forge Execution object key.
	ExecutionObjectKey string `json:"executionObjectKey,omitempty"`
	// PassObjectKey is the optional owning Forge Pass object key.
	PassObjectKey string `json:"passObjectKey,omitempty"`
	// RepoObjectKey is the Git Repo object key.
	RepoObjectKey string `json:"repoObjectKey,omitempty"`
	// WorktreeObjectKey is the allocated Git Worktree object key.
	WorktreeObjectKey string `json:"worktreeObjectKey,omitempty"`
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
	// Timestamp is the allocation timestamp.
	Timestamp *timestamp.Timestamp `json:"timestamp,omitempty"`
}

// CreateArgs contains inputs for creating or reusing an allocation.
type CreateArgs struct {
	ObjectKey          string
	ExecutionObjectKey string
	PassObjectKey      string
	RepoObjectKey      string
	WorktreeObjectKey  string
	BaseCommitHash     string
	BranchRef          string
	PathFamily         string
	EvidenceObjectKey  string
	Status             string
	CollisionState     string
	StaleBaseState     string
	CleanupState       string
	Timestamp          *timestamp.Timestamp
}

// BuildObjectKey builds a deterministic allocation object key.
func BuildObjectKey(executionObjectKey, repoObjectKey, baseCommitHash, pathFamily string) string {
	hash := sha256.Sum256([]byte(strings.Join([]string{
		executionObjectKey,
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
	if args.ObjectKey == "" {
		args.ObjectKey = BuildObjectKey(
			args.ExecutionObjectKey,
			args.RepoObjectKey,
			args.BaseCommitHash,
			args.PathFamily,
		)
	}
	alloc := &Allocation{
		ExecutionObjectKey: args.ExecutionObjectKey,
		PassObjectKey:      args.PassObjectKey,
		RepoObjectKey:      args.RepoObjectKey,
		WorktreeObjectKey:  args.WorktreeObjectKey,
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
	if err := alloc.Validate(); err != nil {
		return nil, "", nil, err
	}

	existing, found, err := ws.GetObject(ctx, args.ObjectKey)
	if err != nil {
		return nil, "", nil, err
	}
	if found {
		existingAlloc, err := Lookup(ctx, ws, args.ObjectKey)
		if err != nil {
			return nil, "", nil, err
		}
		if !existingAlloc.sameAllocation(alloc) {
			return nil, "", nil, errors.Errorf("allocation key collision: %s", args.ObjectKey)
		}
		ref, _, err := existing.GetRootRef(ctx)
		return existingAlloc, args.ObjectKey, ref, err
	}

	_, rootRef, err := world.CreateWorldObject(ctx, ws, args.ObjectKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(alloc, true)
		return nil
	})
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
	switch {
	case a.GetExecutionObjectKey() == "":
		return errors.New("execution_object_key cannot be empty")
	case a.GetRepoObjectKey() == "":
		return errors.New("repo_object_key cannot be empty")
	case a.GetWorktreeObjectKey() == "":
		return errors.New("worktree_object_key cannot be empty")
	case a.GetBaseCommitHash() == "":
		return errors.New("base_commit_hash cannot be empty")
	case a.GetBranchRef() == "":
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
	if err := a.GetTimestamp().Validate(false); err != nil {
		return errors.Wrap(err, "timestamp")
	}
	return nil
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
	if a == nil {
		return []byte("null"), nil
	}

	var arena fastjson.Arena
	obj := arena.NewObject()
	setStringJSONField(&arena, obj, "executionObjectKey", a.ExecutionObjectKey)
	setStringJSONField(&arena, obj, "passObjectKey", a.PassObjectKey)
	setStringJSONField(&arena, obj, "repoObjectKey", a.RepoObjectKey)
	setStringJSONField(&arena, obj, "worktreeObjectKey", a.WorktreeObjectKey)
	setStringJSONField(&arena, obj, "baseCommitHash", a.BaseCommitHash)
	setStringJSONField(&arena, obj, "branchRef", a.BranchRef)
	setStringJSONField(&arena, obj, "pathFamily", a.PathFamily)
	setStringJSONField(&arena, obj, "evidenceObjectKey", a.EvidenceObjectKey)
	setStringJSONField(&arena, obj, "status", a.Status)
	setStringJSONField(&arena, obj, "collisionState", a.CollisionState)
	setStringJSONField(&arena, obj, "staleBaseState", a.StaleBaseState)
	setStringJSONField(&arena, obj, "cleanupState", a.CleanupState)
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

func setStringJSONField(arena *fastjson.Arena, obj *fastjson.Value, key, value string) {
	if value != "" {
		obj.Set(key, arena.NewString(value))
	}
}

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

	a.ExecutionObjectKey = string(value.GetStringBytes("executionObjectKey"))
	a.PassObjectKey = string(value.GetStringBytes("passObjectKey"))
	a.RepoObjectKey = string(value.GetStringBytes("repoObjectKey"))
	a.WorktreeObjectKey = string(value.GetStringBytes("worktreeObjectKey"))
	a.BaseCommitHash = string(value.GetStringBytes("baseCommitHash"))
	a.BranchRef = string(value.GetStringBytes("branchRef"))
	a.PathFamily = string(value.GetStringBytes("pathFamily"))
	a.EvidenceObjectKey = string(value.GetStringBytes("evidenceObjectKey"))
	a.Status = string(value.GetStringBytes("status"))
	a.CollisionState = string(value.GetStringBytes("collisionState"))
	a.StaleBaseState = string(value.GetStringBytes("staleBaseState"))
	a.CleanupState = string(value.GetStringBytes("cleanupState"))
	if timestampValue := value.Get("timestamp"); timestampValue != nil && timestampValue.Type() != fastjson.TypeNull {
		ts := &timestamp.Timestamp{}
		if err := ts.UnmarshalJSON(timestampValue.MarshalTo(nil)); err != nil {
			return errors.Wrap(err, "unmarshal timestamp")
		}
		a.Timestamp = ts
	} else {
		a.Timestamp = nil
	}
	return nil
}

func (a *Allocation) sameAllocation(other *Allocation) bool {
	return a.GetExecutionObjectKey() == other.GetExecutionObjectKey() &&
		a.GetPassObjectKey() == other.GetPassObjectKey() &&
		a.GetRepoObjectKey() == other.GetRepoObjectKey() &&
		a.GetWorktreeObjectKey() == other.GetWorktreeObjectKey() &&
		a.GetBaseCommitHash() == other.GetBaseCommitHash() &&
		a.GetBranchRef() == other.GetBranchRef() &&
		a.GetPathFamily() == other.GetPathFamily() &&
		a.GetCollisionState() == other.GetCollisionState() &&
		a.GetStaleBaseState() == other.GetStaleBaseState() &&
		a.GetCleanupState() == other.GetCleanupState()
}

func setAllocationQuads(ctx context.Context, ws world.WorldState, objKey string, alloc *Allocation) error {
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(alloc.GetExecutionObjectKey(), PredExecutionToAllocation.String(), objKey, "")); err != nil {
		return err
	}
	if passKey := alloc.GetPassObjectKey(); passKey != "" {
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(passKey, PredPassToAllocation.String(), objKey, "")); err != nil {
			return err
		}
	}
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(objKey, PredAllocationToRepo.String(), alloc.GetRepoObjectKey(), "")); err != nil {
		return err
	}
	return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(objKey, PredAllocationToWorktree.String(), alloc.GetWorktreeObjectKey(), ""))
}

// _ is a type assertion
var _ block.Block = ((*Allocation)(nil))

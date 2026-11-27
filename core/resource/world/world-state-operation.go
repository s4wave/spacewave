package resource_world

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// WorldStateOperationRecord describes one WorldStateResource operation.
type WorldStateOperationRecord struct {
	// Name is the WorldStateResource method name.
	Name string
	// Duration is the elapsed wall-clock duration.
	Duration time.Duration
	// Error is set when the operation returned an error.
	Error string
	// FilterCount is the number of graph filters in the request.
	FilterCount int
	// Limit is the relevant request limit.
	Limit int
	// ResultSetCount is the number of result groups returned.
	ResultSetCount int
	// ResultQuadCount is the total number of graph quads returned.
	ResultQuadCount int
	// StartKeyCount is the number of graph path start keys.
	StartKeyCount int
	// StepCount is the number of graph path steps.
	StepCount int
	// PageSize is the requested page size for resource-backed operations.
	PageSize int
	// ResultObjectCount is the number of object keys returned by the operation.
	ResultObjectCount int
	// ResourceCreated indicates that the operation attached a resource.
	ResourceCreated bool
	// ReadCounterSnapshot is the block read-counter snapshot for the operation.
	ReadCounterSnapshot block.ReadCounterSnapshot
}

// WorldStateOperationObserver observes WorldStateResource operation records.
type WorldStateOperationObserver func(WorldStateOperationRecord)

// mountOptions holds trusted configuration independently of any live Resource.
type mountOptions struct {
	// options retains the trusted options propagated into descendants.
	options []WorldStateResourceOption
	// outerEngine is the Space Engine that authorized a nested World mount.
	outerEngine world.Engine

	// engineID is the registry scope granted by the mounting component.
	engineID string
	// engineIDBound prevents incoming caller context from changing registry scope.
	engineIDBound bool
	// sessionPeerID is the authenticated peer selected by the granting mount.
	sessionPeerID peer.ID
	// sessionPeerIDBound prevents request context from replacing the mount peer.
	sessionPeerIDBound bool
	// participantLookup reads the caller's accepted participant, when known.
	participantLookup func(context.Context, peer.ID) (*sobject.SOParticipantConfig, error)
	// operationObserver receives operation accounting for this mount.
	operationObserver WorldStateOperationObserver
	// writeGuard checks every write through a mounted capability, when set.
	writeGuard world.WriteGuard
}

// WorldStateResourceOption configures trusted access inherited by mounted Resources.
type WorldStateResourceOption func(*mountOptions)

// WithWorldStateOperationObserver configures WorldStateResource operation accounting.
func WithWorldStateOperationObserver(observer WorldStateOperationObserver) WorldStateResourceOption {
	return func(r *mountOptions) {
		r.operationObserver = observer
	}
}

// WithSessionPeerID binds typed access and operation execution to the authenticated session peer.
func WithSessionPeerID(sessionPeerID peer.ID) WorldStateResourceOption {
	return func(r *mountOptions) {
		r.sessionPeerID = sessionPeerID
		r.sessionPeerIDBound = true
	}
}

// WithCallerParticipantLookup reads accepted caller identity from the granting mount.
// A nil lookup leaves the accepted participant unknown.
func WithCallerParticipantLookup(lookup func(context.Context, peer.ID) (*sobject.SOParticipantConfig, error)) WorldStateResourceOption {
	return func(r *mountOptions) { r.participantLookup = lookup }
}

// WithEngineID binds typed access to the registry scope selected by the
// granting mount. An empty ID binds global scope. Callers cannot override a
// trusted binding.
func WithEngineID(engineID string) WorldStateResourceOption {
	return func(r *mountOptions) {
		r.engineID = engineID
		r.engineIDBound = true
	}
}

// WithOuterEngine grants OpenOuterWorld the Space Engine that authorized a
// nested World mount. A nil Engine marks a top-level mount.
func WithOuterEngine(engine world.Engine) WorldStateResourceOption {
	return func(r *mountOptions) { r.outerEngine = engine }
}

// WithWriteGuard checks every write through the mount, and through every World,
// object and typed handle opened under it, with guard first.
func WithWriteGuard(guard world.WriteGuard) WorldStateResourceOption {
	return func(r *mountOptions) { r.writeGuard = guard }
}

// newMountOptions applies trusted options in order, preserving the last binding.
func newMountOptions(opts ...WorldStateResourceOption) mountOptions {
	r := mountOptions{options: opts}
	for _, opt := range opts {
		if opt != nil {
			opt(&r)
		}
	}
	return r
}

// lookupParticipant reads the accepted caller while preserving a non-participant mount.
func (r *mountOptions) lookupParticipant(ctx context.Context) (*sobject.SOParticipantConfig, error) {
	// A mount without an authenticated participant lookup has unknown membership.
	if r.participantLookup == nil || r.sessionPeerID == "" {
		return nil, nil
	}

	// Keep a non-participant response as metadata rather than rejecting the mount.
	participant, err := r.participantLookup(ctx, r.sessionPeerID)
	if err != nil && !errors.Is(err, sobject.ErrNotParticipant) {
		return nil, err
	}
	return participant, nil
}

// resolveWorldOp resolves and decodes the operation req specifies and binds its
// sender to the authenticated mount when one is bound. It returns a refusal for
// an operation type the mount cannot resolve.
func (r *mountOptions) resolveWorldOp(
	ctx context.Context,
	lookupOp world.LookupOp,
	req *s4wave_world.ApplyWorldOpRequest,
) (world.Operation, peer.ID, *s4wave_world.ApplyWorldOpResponse, error) {
	// Refuse an op the mount cannot resolve without an error.
	unhandled := &s4wave_world.ApplyWorldOpResponse{
		ErrorCode: s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP,
	}
	if lookupOp == nil {
		return nil, "", unhandled, nil
	}

	// Resolve the requested World operation and report unsupported types.
	op, err := lookupOp(ctx, req.GetOpTypeId())
	if err == nil && op == nil {
		err = world.ErrUnhandledOp
	}
	if err != nil {
		if errors.Is(err, world.ErrUnhandledOp) {
			return nil, "", unhandled, nil
		}
		return nil, "", nil, err
	}

	// Decode the World operation payload before execution.
	if err := op.UnmarshalVT(req.GetOpData()); err != nil {
		return nil, "", nil, err
	}

	// Bind the operation sender to the authenticated mount when available.
	opSender := r.sessionPeerID
	if !r.sessionPeerIDBound {
		opSender, err = req.ParsePeerID()
		if err != nil {
			return nil, "", nil, err
		}
	}
	return op, opSender, nil, nil
}

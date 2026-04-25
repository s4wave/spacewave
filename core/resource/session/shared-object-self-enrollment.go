package resource_session

import (
	"context"
	"slices"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// SharedObjectSelfEnrollmentResource wraps post-sign-in self-enrollment state.
type SharedObjectSelfEnrollmentResource struct {
	mux   srpc.Invoker
	swAcc *provider_spacewave.ProviderAccount

	// bcast guards run state fields below.
	bcast                 broadcast.Broadcast
	running               bool
	currentSharedObjectID string
	completedIDs          []string
	failures              []*s4wave_session.SharedObjectSelfEnrollmentFailure
}

// NewSharedObjectSelfEnrollmentResource creates a new SharedObjectSelfEnrollmentResource.
func NewSharedObjectSelfEnrollmentResource(
	swAcc *provider_spacewave.ProviderAccount,
) *SharedObjectSelfEnrollmentResource {
	r := &SharedObjectSelfEnrollmentResource{
		swAcc: swAcc,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_session.SRPCRegisterSharedObjectSelfEnrollmentResourceService(mux, r)
	})
	return r
}

// GetMux returns the rpc mux.
func (r *SharedObjectSelfEnrollmentResource) GetMux() srpc.Invoker {
	return r.mux
}

// WatchState streams self-enrollment state changes.
func (r *SharedObjectSelfEnrollmentResource) WatchState(
	req *s4wave_session.WatchSharedObjectSelfEnrollmentStateRequest,
	strm s4wave_session.SRPCSharedObjectSelfEnrollmentResourceService_WatchStateStream,
) error {
	ctx := strm.Context()
	accountBcast := r.swAcc.GetAccountBroadcast()

	var prev *s4wave_session.WatchSharedObjectSelfEnrollmentStateResponse
	for {
		var accountCh <-chan struct{}
		var summary *provider_spacewave.SelfEnrollmentSummary
		var skippedKey string
		accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			accountCh = getWaitCh()
			summary = r.swAcc.GetSelfEnrollmentSummary()
			skippedKey = r.swAcc.GetSelfEnrollmentSkippedGenerationKey()
		})

		var resourceCh <-chan struct{}
		var running bool
		var current string
		var completed []string
		var failures []*s4wave_session.SharedObjectSelfEnrollmentFailure
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			resourceCh = getWaitCh()
			running = r.running
			current = r.currentSharedObjectID
			completed = slices.Clone(r.completedIDs)
			failures = cloneSelfEnrollmentFailures(r.failures)
		})

		resp := r.buildStateResponse(summary, running, current, completed, failures, skippedKey)
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-accountCh:
		case <-resourceCh:
		}
	}
}

// Start runs self-enrollment for the current pending set.
func (r *SharedObjectSelfEnrollmentResource) Start(
	ctx context.Context,
	req *s4wave_session.StartSharedObjectSelfEnrollmentRequest,
) (*s4wave_session.StartSharedObjectSelfEnrollmentResponse, error) {
	store := r.swAcc.GetEntityKeyStore()
	if store == nil || len(store.GetUnlockedKeys()) == 0 {
		return nil, sobject.ErrSharedObjectRecoveryCredentialRequired
	}
	ref := r.swAcc.RetainEntityKeypairStepUp()
	defer ref.Release()

	var summary *provider_spacewave.SelfEnrollmentSummary
	accountBcast := r.swAcc.GetAccountBroadcast()
	accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		summary = r.swAcc.GetSelfEnrollmentSummary()
	})
	if summary == nil || summary.GetCount() == 0 {
		return &s4wave_session.StartSharedObjectSelfEnrollmentResponse{}, nil
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.running = true
		r.currentSharedObjectID = ""
		r.completedIDs = nil
		r.failures = nil
		broadcast()
	})
	defer r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.running = false
		r.currentSharedObjectID = ""
		broadcast()
	})

	for _, soID := range summary.GetIDs() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.currentSharedObjectID = soID
			broadcast()
		})
		ref := sobject.NewSharedObjectRef(
			r.swAcc.GetProviderID(),
			r.swAcc.GetAccountID(),
			soID,
			provider_spacewave.SobjectBlockStoreID(soID),
		)
		_, rel, err := r.swAcc.MountSharedObject(ctx, ref, nil)
		if rel != nil {
			rel()
		}
		if err != nil {
			failure := &s4wave_session.SharedObjectSelfEnrollmentFailure{
				SharedObjectId: soID,
				Category:       categorizeSelfEnrollmentError(err),
				Message:        err.Error(),
			}
			r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				r.failures = append(r.failures, failure)
				broadcast()
			})
			continue
		}
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.completedIDs = append(r.completedIDs, soID)
			broadcast()
		})
	}
	if err := r.swAcc.RefreshSelfEnrollmentSummary(ctx); err != nil {
		return nil, err
	}
	return &s4wave_session.StartSharedObjectSelfEnrollmentResponse{}, nil
}

// Skip records the user's skip choice for the current generation.
func (r *SharedObjectSelfEnrollmentResource) Skip(
	ctx context.Context,
	req *s4wave_session.SkipSharedObjectSelfEnrollmentRequest,
) (*s4wave_session.SkipSharedObjectSelfEnrollmentResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := req.GetGenerationKey()
	if key == "" {
		accountBcast := r.swAcc.GetAccountBroadcast()
		accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if summary := r.swAcc.GetSelfEnrollmentSummary(); summary != nil {
				key = summary.GetGenerationKey()
			}
		})
	}
	r.swAcc.SetSelfEnrollmentSkippedGenerationKey(key)
	return &s4wave_session.SkipSharedObjectSelfEnrollmentResponse{}, nil
}

func (r *SharedObjectSelfEnrollmentResource) buildStateResponse(
	summary *provider_spacewave.SelfEnrollmentSummary,
	running bool,
	current string,
	completed []string,
	failures []*s4wave_session.SharedObjectSelfEnrollmentFailure,
	skippedKey string,
) *s4wave_session.WatchSharedObjectSelfEnrollmentStateResponse {
	resp := &s4wave_session.WatchSharedObjectSelfEnrollmentStateResponse{
		Running:                  running,
		CurrentSharedObjectId:    current,
		CompletedSharedObjectIds: completed,
		Failures:                 failures,
		SkippedGenerationKey:     skippedKey,
	}
	if summary == nil {
		return resp
	}
	resp.SharedObjectIds = summary.GetIDs()
	resp.GenerationKey = summary.GetGenerationKey()
	resp.Count = summary.GetCount()
	store := r.swAcc.GetEntityKeyStore()
	resp.CredentialRequired = summary.GetCount() != 0 &&
		(store == nil || len(store.GetUnlockedKeys()) == 0)
	resp.Skipped = skippedKey != "" && skippedKey == summary.GetGenerationKey()
	return resp
}

func categorizeSelfEnrollmentError(err error) s4wave_session.SharedObjectSelfEnrollmentErrorCategory {
	if errors.Is(err, sobject.ErrSharedObjectRecoveryCredentialRequired) {
		return s4wave_session.SharedObjectSelfEnrollmentErrorCategory_SHARED_OBJECT_SELF_ENROLLMENT_ERROR_CATEGORY_RETRY
	}
	if errors.Is(err, sobject.ErrNotParticipant) {
		return s4wave_session.SharedObjectSelfEnrollmentErrorCategory_SHARED_OBJECT_SELF_ENROLLMENT_ERROR_CATEGORY_OPEN_OBJECT
	}
	return s4wave_session.SharedObjectSelfEnrollmentErrorCategory_SHARED_OBJECT_SELF_ENROLLMENT_ERROR_CATEGORY_REPORT
}

func cloneSelfEnrollmentFailures(
	failures []*s4wave_session.SharedObjectSelfEnrollmentFailure,
) []*s4wave_session.SharedObjectSelfEnrollmentFailure {
	if len(failures) == 0 {
		return nil
	}
	next := make([]*s4wave_session.SharedObjectSelfEnrollmentFailure, len(failures))
	for i, failure := range failures {
		if failure != nil {
			next[i] = failure.CloneVT()
		}
	}
	return next
}

// _ is a type assertion
var _ s4wave_session.SRPCSharedObjectSelfEnrollmentResourceServiceServer = ((*SharedObjectSelfEnrollmentResource)(nil))

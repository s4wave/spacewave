package resource_session

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
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

		run, runCh := r.swAcc.WatchSelfEnrollmentRunSnapshot()
		store := r.swAcc.GetEntityKeyStore()
		var entityCh <-chan struct{}
		unlockedCount := 0
		if store != nil {
			unlockedCount, entityCh = store.WatchUnlockedCount()
		}

		resp := r.buildStateResponse(summary, run, skippedKey, store != nil, unlockedCount)
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
		case <-runCh:
		case <-entityCh:
		}
	}
}

// Start runs self-enrollment for the current pending set.
func (r *SharedObjectSelfEnrollmentResource) Start(
	ctx context.Context,
	req *s4wave_session.StartSharedObjectSelfEnrollmentRequest,
) (*s4wave_session.StartSharedObjectSelfEnrollmentResponse, error) {
	if err := r.swAcc.StartSelfEnrollmentRun(ctx); err != nil {
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
	run *provider_spacewave.SelfEnrollmentRunSnapshot,
	skippedKey string,
	hasEntityKeyStore bool,
	unlockedCount int,
) *s4wave_session.WatchSharedObjectSelfEnrollmentStateResponse {
	resp := &s4wave_session.WatchSharedObjectSelfEnrollmentStateResponse{
		SkippedGenerationKey: skippedKey,
	}
	if run != nil {
		resp.Running = run.Running
		resp.CurrentSharedObjectId = run.CurrentSharedObjectID
		resp.CompletedSharedObjectIds = run.CompletedIDs
		resp.Failures = buildSelfEnrollmentFailures(run.Failures)
	}
	if summary == nil {
		return resp
	}
	resp.SharedObjectIds = summary.GetIDs()
	resp.GenerationKey = summary.GetGenerationKey()
	resp.Count = summary.GetCount()
	resp.CredentialRequired = summary.GetCount() != 0 &&
		(!hasEntityKeyStore || unlockedCount == 0)
	resp.Skipped = skippedKey != "" && skippedKey == summary.GetGenerationKey()
	return resp
}

func buildSelfEnrollmentFailures(
	failures []*provider_spacewave.SelfEnrollmentRunFailure,
) []*s4wave_session.SharedObjectSelfEnrollmentFailure {
	if len(failures) == 0 {
		return nil
	}
	out := make([]*s4wave_session.SharedObjectSelfEnrollmentFailure, 0, len(failures))
	for _, failure := range failures {
		if failure == nil {
			continue
		}
		message := ""
		if failure.Err != nil {
			message = failure.Err.Error()
		}
		out = append(out, &s4wave_session.SharedObjectSelfEnrollmentFailure{
			SharedObjectId: failure.SharedObjectID,
			Category:       categorizeSelfEnrollmentError(failure.Err),
			Message:        message,
		})
	}
	return out
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

// _ is a type assertion
var _ s4wave_session.SRPCSharedObjectSelfEnrollmentResourceServiceServer = ((*SharedObjectSelfEnrollmentResource)(nil))

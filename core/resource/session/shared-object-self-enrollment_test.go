//go:build !goscript

package resource_session

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

func TestSharedObjectSelfEnrollmentStartRequiresCredential(t *testing.T) {
	res := NewSharedObjectSelfEnrollmentResource(&provider_spacewave.ProviderAccount{})
	_, err := res.Start(context.Background(), &s4wave_session.StartSharedObjectSelfEnrollmentRequest{})
	if !errors.Is(err, sobject.ErrSharedObjectRecoveryCredentialRequired) {
		t.Fatalf("Start error = %v, want credential required", err)
	}
}

func TestSharedObjectSelfEnrollmentSkipRecordsGeneration(t *testing.T) {
	acc := &provider_spacewave.ProviderAccount{}
	res := NewSharedObjectSelfEnrollmentResource(acc)
	_, err := res.Skip(context.Background(), &s4wave_session.SkipSharedObjectSelfEnrollmentRequest{
		GenerationKey: "gen-1",
	})
	if err != nil {
		t.Fatalf("Skip: %v", err)
	}
	if got := acc.GetSelfEnrollmentSkippedGenerationKey(); got != "gen-1" {
		t.Fatalf("skipped generation = %q, want gen-1", got)
	}
}

func TestSharedObjectSelfEnrollmentCategorizesFailures(t *testing.T) {
	if got := categorizeSelfEnrollmentError(sobject.ErrSharedObjectRecoveryCredentialRequired); got != s4wave_session.SharedObjectSelfEnrollmentErrorCategory_SHARED_OBJECT_SELF_ENROLLMENT_ERROR_CATEGORY_RETRY {
		t.Fatalf("credential category = %v", got)
	}
	if got := categorizeSelfEnrollmentError(sobject.ErrNotParticipant); got != s4wave_session.SharedObjectSelfEnrollmentErrorCategory_SHARED_OBJECT_SELF_ENROLLMENT_ERROR_CATEGORY_OPEN_OBJECT {
		t.Fatalf("not participant category = %v", got)
	}
	if got := categorizeSelfEnrollmentError(errors.New("boom")); got != s4wave_session.SharedObjectSelfEnrollmentErrorCategory_SHARED_OBJECT_SELF_ENROLLMENT_ERROR_CATEGORY_REPORT {
		t.Fatalf("generic category = %v", got)
	}
}

func TestSharedObjectSelfEnrollmentStateResponseSerializesProjectionAgreement(t *testing.T) {
	resp := buildStateResponse(&provider_spacewave.SelfEnrollmentProjection{
		SharedObjectIDs:          []string{"so-1", "so-2"},
		GenerationKey:            "gen-1",
		Count:                    2,
		CredentialRequired:       true,
		Running:                  true,
		CurrentSharedObjectID:    "so-2",
		CompletedSharedObjectIDs: []string{"so-1"},
		Skipped:                  true,
		SkippedGenerationKey:     "gen-1",
		Failures: []*provider_spacewave.SelfEnrollmentRunFailure{{
			SharedObjectID: "so-3",
			Err:            sobject.ErrNotParticipant,
		}},
	})

	data, err := resp.MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT: %v", err)
	}
	var got s4wave_session.WatchSharedObjectSelfEnrollmentStateResponse
	if err := got.UnmarshalVT(data); err != nil {
		t.Fatalf("UnmarshalVT: %v", err)
	}
	if !resp.EqualVT(&got) {
		t.Fatalf("round trip response = %+v, want %+v", &got, resp)
	}
	if got.GetGenerationKey() != "gen-1" ||
		got.GetCount() != 2 ||
		!got.GetCredentialRequired() ||
		!got.GetRunning() ||
		got.GetCurrentSharedObjectId() != "so-2" ||
		!got.GetSkipped() ||
		got.GetSkippedGenerationKey() != "gen-1" {
		t.Fatalf("decoded response drifted: %+v", &got)
	}
	if len(got.GetSharedObjectIds()) != 2 ||
		got.GetSharedObjectIds()[0] != "so-1" ||
		got.GetSharedObjectIds()[1] != "so-2" {
		t.Fatalf("shared object ids = %#v", got.GetSharedObjectIds())
	}
	if len(got.GetCompletedSharedObjectIds()) != 1 ||
		got.GetCompletedSharedObjectIds()[0] != "so-1" {
		t.Fatalf("completed ids = %#v", got.GetCompletedSharedObjectIds())
	}
	if len(got.GetFailures()) != 1 ||
		got.GetFailures()[0].GetSharedObjectId() != "so-3" ||
		got.GetFailures()[0].GetCategory() != s4wave_session.SharedObjectSelfEnrollmentErrorCategory_SHARED_OBJECT_SELF_ENROLLMENT_ERROR_CATEGORY_OPEN_OBJECT ||
		got.GetFailures()[0].GetMessage() != sobject.ErrNotParticipant.Error() {
		t.Fatalf("failures = %+v", got.GetFailures())
	}
}

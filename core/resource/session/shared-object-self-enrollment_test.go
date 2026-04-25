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

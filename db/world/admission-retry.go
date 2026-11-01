package world

import "context"

// AdmissionRetry is an unadmitted attempt that needs preparation outside every
// caller writer. Discard the attempt before Prepare, then rerun its retry-safe
// body in a fresh transaction. A failed predecessor or retired engine ends the
// preparation with an error instead of reusing that attempt's assumptions.
type AdmissionRetry interface {
	error
	// Prepare waits without caller writers and reserves the next attempt.
	Prepare(context.Context) (AdmissionPreparation, error)
}

// AdmissionPreparation owns one attempt's pre-lock admission resources. Pass its
// context to NewTransaction, the body and CommitAdmit, and Release after Discard,
// including when opening the transaction fails. It retains no candidate data.
type AdmissionPreparation interface {
	// WithContext carries preparation to the engine that produced it.
	WithContext(context.Context) context.Context
	// Release returns unused capacity and preparation ownership once.
	Release()
}

// admissionRetryKey marks a caller that can discard and rerun its write body.
type admissionRetryKey struct{}

// WithAdmissionRetry asks CommitAdmit to return AdmissionRetry instead of waiting
// for admission preparation while the caller holds an independent writer.
func WithAdmissionRetry(ctx context.Context) context.Context {
	return context.WithValue(ctx, admissionRetryKey{}, true)
}

// CanRetryAdmission reports whether the caller accepts an unadmitted retry.
func CanRetryAdmission(ctx context.Context) bool {
	retry, _ := ctx.Value(admissionRetryKey{}).(bool)
	return retry
}

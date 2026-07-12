//go:build js

package opfs

import "strconv"

// QuotaExceededError reports a browser origin-storage limit with the latest
// available usage, quota, and pending-write size.
type QuotaExceededError struct {
	Usage    uint64
	Quota    uint64
	Required uint64
	Cause    error
}

// Error implements error.
func (e *QuotaExceededError) Error() string {
	msg := "browser storage quota exceeded"
	if e.Quota != 0 {
		available := uint64(0)
		if e.Quota > e.Usage {
			available = e.Quota - e.Usage
		}
		msg += " (usage " + strconv.FormatUint(e.Usage, 10) +
			" bytes, quota " + strconv.FormatUint(e.Quota, 10) +
			" bytes, available " + strconv.FormatUint(available, 10) + " bytes"
		if e.Required != 0 {
			msg += ", write " + strconv.FormatUint(e.Required, 10) + " bytes"
		}
		msg += ")"
	}
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

// Unwrap returns the browser error that caused the quota failure.
func (e *QuotaExceededError) Unwrap() error {
	return e.Cause
}

// WithQuotaEstimate promotes a browser QuotaExceededError to the typed storage
// error and attaches the latest available StorageManager estimate.
func WithQuotaEstimate(err error, required uint64) error {
	if !IsQuotaExceeded(err) {
		return err
	}
	quotaErr := &QuotaExceededError{Required: required, Cause: err}
	if estimate, estimateErr := EstimateStorage(); estimateErr == nil {
		quotaErr.Usage = estimate.Usage
		quotaErr.Quota = estimate.Quota
	}
	return quotaErr
}

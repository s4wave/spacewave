//go:build !tinygo

package block_store_s3

// PackVerifyError is the error VerifyPack returns when a pack fails, with
// one of the Verify reasons.
type PackVerifyError struct {
	// Reason is one of the Verify constants.
	Reason string
	// Err is the cause, if any.
	Err error
}

// Error describes the failure.
func (e *PackVerifyError) Error() string {
	if e.Err == nil {
		return "pack verify: " + e.Reason
	}
	return "pack verify: " + e.Reason + ": " + e.Err.Error()
}

// Unwrap returns the cause.
func (e *PackVerifyError) Unwrap() error { return e.Err }

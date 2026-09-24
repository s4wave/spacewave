//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
)

// checkProbeData is the body of the probe object the check writes.
var checkProbeData = []byte("spacewave storage check\n")

// CheckBucket writes, reads back, and deletes a probe object under
// objectPrefix, and classifies the first failure. Every step a block store
// performs must succeed for the result to be CHECK_OUTCOME_OK.
func CheckBucket(ctx context.Context, client *Client, bucket, objectPrefix string) *CheckResult {
	key := objectPrefix + ".spacewave-check/" + ulid.NewULID()

	// Write the probe object.
	if err := client.PutObject(ctx, bucket, key, checkProbeData, "text/plain"); err != nil {
		return newCheckFailure("write", err)
	}

	// Read the probe object back and compare its body.
	body, err := client.GetObject(ctx, bucket, key)
	if err != nil {
		return newCheckFailure("read", err)
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return newCheckFailure("read", err)
	}
	if !bytes.Equal(data, checkProbeData) {
		return &CheckResult{
			Outcome: CheckOutcome_CHECK_OUTCOME_FAILED,
			Detail:  "read: the bucket returned different bytes than were written",
		}
	}

	// Delete the probe object.
	if err := client.DeleteObject(ctx, bucket, key); err != nil && !errors.Is(err, ErrNotFound) {
		return newCheckFailure("delete", err)
	}
	return &CheckResult{Outcome: CheckOutcome_CHECK_OUTCOME_OK}
}

// newCheckFailure classifies the error from a failed check step.
func newCheckFailure(step string, err error) *CheckResult {
	return &CheckResult{
		Outcome: ClassifyError(err),
		Detail:  step + ": " + err.Error(),
	}
}

// ClassifyError maps an error from the S3 client to a CheckOutcome.
func ClassifyError(err error) CheckOutcome {
	if errors.Is(err, ErrBucketNotFound) {
		return CheckOutcome_CHECK_OUTCOME_BUCKET_NOT_FOUND
	}
	var serr *StatusError
	if !errors.As(err, &serr) {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return CheckOutcome_CHECK_OUTCOME_FAILED
		}
		return CheckOutcome_CHECK_OUTCOME_UNREACHABLE
	}
	switch serr.Code {
	case "InvalidAccessKeyId", "SignatureDoesNotMatch", "InvalidToken", "ExpiredToken":
		return CheckOutcome_CHECK_OUTCOME_CREDENTIALS_REJECTED
	case "PermanentRedirect", "AuthorizationHeaderMalformed", "IllegalLocationConstraintException":
		return CheckOutcome_CHECK_OUTCOME_WRONG_REGION
	}
	switch serr.StatusCode {
	case http.StatusForbidden:
		return CheckOutcome_CHECK_OUTCOME_ACCESS_DENIED
	case http.StatusMovedPermanently:
		return CheckOutcome_CHECK_OUTCOME_WRONG_REGION
	}
	return CheckOutcome_CHECK_OUTCOME_FAILED
}

//go:build !tinygo

package block_store_ftp

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/sirupsen/logrus"
)

// checkStoreID is the block store id of the probe block. The probe is never
// deleted, since the store only adds files, and a repeated check writes the
// same file.
const checkStoreID = ".spacewave-check"

// checkProbeData is the data of the probe block the check writes.
var checkProbeData = []byte("spacewave storage check\n")

// Check writes the probe block under the root and reads it back as the store
// does, classifying the first failure.
func Check(ctx context.Context, conf *Config) *block_store_s3.CheckResult {
	// Open the probe store under the root.
	store, err := NewStore(logrus.NewEntry(logrus.StandardLogger()), NewClient(conf), checkStoreID)
	if err != nil {
		return checkFailure("open", err)
	}
	defer store.Close()

	// Write the probe block and read it back.
	ref, err := block.BuildBlockRef(checkProbeData, nil)
	if err != nil {
		return checkFailure("write", err)
	}
	if _, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: checkProbeData, RefsKnown: true}}); err != nil {
		return checkFailure("write", err)
	}
	data, found, err := store.GetBlock(ctx, ref)
	if err != nil {
		return checkFailure("read", err)
	}
	if !found || !bytes.Equal(data, checkProbeData) {
		return &block_store_s3.CheckResult{
			Outcome: block_store_s3.CheckOutcome_CHECK_OUTCOME_FAILED,
			Detail:  "read: the server returned different bytes than were written",
		}
	}
	return &block_store_s3.CheckResult{Outcome: block_store_s3.CheckOutcome_CHECK_OUTCOME_OK}
}

// checkFailure classifies the failure of a check step.
func checkFailure(step string, err error) *block_store_s3.CheckResult {
	// Classify the reply code, then the certificate, then the network.
	outcome := block_store_s3.CheckOutcome_CHECK_OUTCOME_FAILED
	var nerr net.Error
	var cerr *tls.CertificateVerificationError
	var uerr x509.UnknownAuthorityError
	switch code := replyCode(err); {
	case code == ftp.StatusNotLoggedIn:
		outcome = block_store_s3.CheckOutcome_CHECK_OUTCOME_CREDENTIALS_REJECTED
	case code == ftp.StatusFileUnavailable || code == ftp.StatusBadFileName:
		outcome = block_store_s3.CheckOutcome_CHECK_OUTCOME_ACCESS_DENIED
	case errors.As(err, &cerr) || errors.As(err, &uerr):
		// A certificate the system does not trust stays a plain failure.
	case errors.As(err, &nerr):
		outcome = block_store_s3.CheckOutcome_CHECK_OUTCOME_UNREACHABLE
	}
	return &block_store_s3.CheckResult{Outcome: outcome, Detail: step + ": " + err.Error()}
}

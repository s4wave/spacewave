//go:build !tinygo

package block_store_sql

import (
	"bytes"
	"context"
	"net"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/sirupsen/logrus"
)

// checkStoreID is the block store id of the probe row. The probe is never
// deleted, since the store's user may only add rows, and a repeated check
// writes the same row.
const checkStoreID = ".spacewave-check"

// checkProbeData is the data of the probe block the check writes.
var checkProbeData = []byte("spacewave storage check\n")

// Check creates the blocks table when missing, then writes the probe block and
// reads it back as the store does, classifying the first failure.
func Check(ctx context.Context, conf *Config) *block_store_s3.CheckResult {
	// Open the database and create the table.
	db, err := Open(conf)
	if err != nil {
		return checkFailure("connect", err)
	}
	store := NewStore(logrus.NewEntry(logrus.StandardLogger()), db, conf.Table, checkStoreID)
	defer store.Close()
	if err := EnsureTable(ctx, db, conf.Table); err != nil {
		return checkFailure("create table", err)
	}

	// Write the probe block and read it back.
	ref, _, err := store.PutBlock(ctx, checkProbeData, nil)
	if err != nil {
		return checkFailure("write", err)
	}
	data, found, err := store.GetBlock(ctx, ref)
	if err != nil {
		return checkFailure("read", err)
	}
	if !found || !bytes.Equal(data, checkProbeData) {
		return &block_store_s3.CheckResult{
			Outcome: block_store_s3.CheckOutcome_CHECK_OUTCOME_FAILED,
			Detail:  "read: the database returned different bytes than were written",
		}
	}
	return &block_store_s3.CheckResult{Outcome: block_store_s3.CheckOutcome_CHECK_OUTCOME_OK}
}

// checkFailure classifies the failure of a check step.
func checkFailure(step string, err error) *block_store_s3.CheckResult {
	// Map the server's error number, or a network error, to an outcome.
	outcome := block_store_s3.CheckOutcome_CHECK_OUTCOME_FAILED
	var merr *mysql.MySQLError
	var nerr net.Error
	switch {
	case errors.As(err, &merr) && merr.Number == 1045:
		outcome = block_store_s3.CheckOutcome_CHECK_OUTCOME_CREDENTIALS_REJECTED
	case errors.As(err, &merr) && (merr.Number == 1044 || merr.Number == 1142):
		outcome = block_store_s3.CheckOutcome_CHECK_OUTCOME_ACCESS_DENIED
	case errors.As(err, &nerr):
		outcome = block_store_s3.CheckOutcome_CHECK_OUTCOME_UNREACHABLE
	}
	return &block_store_s3.CheckResult{Outcome: outcome, Detail: step + ": " + err.Error()}
}

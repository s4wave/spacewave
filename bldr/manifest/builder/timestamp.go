//go:build !js

package bldr_manifest_builder

import (
	"context"
	"os"
	"strconv"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
)

const sourceDateEpochEnv = "SOURCE_DATE_EPOCH"

// WithManifestCommitTimestampFromEnvironment fixes manifest construction time
// for one builder lifecycle when SOURCE_DATE_EPOCH is present.
func WithManifestCommitTimestampFromEnvironment(ctx context.Context) (context.Context, error) {
	// Read SOURCE_DATE_EPOCH; without it the manifest uses the current time.
	value, ok := os.LookupEnv(sourceDateEpochEnv)
	if !ok {
		return ctx, nil
	}

	// Fix the manifest construction time to the parsed epoch.
	ts, err := parseSourceDateEpoch(value)
	if err != nil {
		return nil, err
	}
	return withManifestCommitTimestamp(ctx, ts), nil
}

func parseSourceDateEpoch(value string) (*timestamp.Timestamp, error) {
	// Parse the epoch as a signed integer of seconds.
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "parse "+sourceDateEpochEnv)
	}

	// Reject a negative epoch, which predates the timestamp type's range.
	if seconds < 0 {
		return nil, errors.New(sourceDateEpochEnv + " must not be negative")
	}

	// Build the timestamp and require it to be a valid proto time.
	ts := timestamp.New(time.Unix(seconds, 0).UTC())
	if err := ts.CheckValid(); err != nil {
		return nil, errors.Wrap(err, "validate "+sourceDateEpochEnv)
	}
	return ts, nil
}

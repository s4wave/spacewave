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
	value, ok := os.LookupEnv(sourceDateEpochEnv)
	if !ok {
		return ctx, nil
	}
	ts, err := parseSourceDateEpoch(value)
	if err != nil {
		return nil, err
	}
	return withManifestCommitTimestamp(ctx, ts), nil
}

func parseSourceDateEpoch(value string) (*timestamp.Timestamp, error) {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "parse "+sourceDateEpochEnv)
	}
	if seconds < 0 {
		return nil, errors.New(sourceDateEpochEnv + " must not be negative")
	}
	ts := timestamp.New(time.Unix(seconds, 0).UTC())
	if err := ts.CheckValid(); err != nil {
		return nil, errors.Wrap(err, "validate "+sourceDateEpochEnv)
	}
	return ts, nil
}

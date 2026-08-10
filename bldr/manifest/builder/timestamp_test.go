//go:build !js

package bldr_manifest_builder

import (
	"context"
	"strings"
	"testing"
)

func TestManifestCommitTimestampFromSourceDateEpoch(t *testing.T) {
	t.Setenv(sourceDateEpochEnv, "1700000000")
	ctx, err := WithManifestCommitTimestampFromEnvironment(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ts := manifestCommitTimestamp(ctx)
	if ts.GetSeconds() != 1700000000 || ts.GetNanos() != 0 {
		t.Fatalf("manifest timestamp = %d.%09d, want 1700000000.000000000", ts.GetSeconds(), ts.GetNanos())
	}
}

func TestManifestCommitTimestampRejectsMalformedSourceDateEpoch(t *testing.T) {
	t.Setenv(sourceDateEpochEnv, "not-a-timestamp")
	_, err := WithManifestCommitTimestampFromEnvironment(context.Background())
	if err == nil || !strings.Contains(err.Error(), "parse SOURCE_DATE_EPOCH") {
		t.Fatalf("error = %v, want SOURCE_DATE_EPOCH parse failure", err)
	}
}

func TestManifestCommitTimestampRejectsNegativeSourceDateEpoch(t *testing.T) {
	t.Setenv(sourceDateEpochEnv, "-1")
	_, err := WithManifestCommitTimestampFromEnvironment(context.Background())
	if err == nil || !strings.Contains(err.Error(), "SOURCE_DATE_EPOCH must not be negative") {
		t.Fatalf("error = %v, want negative SOURCE_DATE_EPOCH failure", err)
	}
}

func TestManifestCommitTimestampRejectsOutOfRangeSourceDateEpoch(t *testing.T) {
	t.Setenv(sourceDateEpochEnv, "9223372036854775807")
	_, err := WithManifestCommitTimestampFromEnvironment(context.Background())
	if err == nil || !strings.Contains(err.Error(), "validate SOURCE_DATE_EPOCH") {
		t.Fatalf("error = %v, want SOURCE_DATE_EPOCH validation failure", err)
	}
}

//go:build !tinygo

package block_store_s3

import (
	"context"
	"strings"

	"github.com/pkg/errors"
)

// A hide keeps a pack's bytes only as long as the bucket keeps the hidden
// version, and a lifecycle rule deletes it on its own: B2's
// daysFromHidingToDeleting deletes a version that many days after a marker
// covered it, and its console's "keep only the last version" is that rule
// with one day on every key. Nothing the hide writes can stop such a rule.
//
// Invariant: a pack sweep hides a pack only with a HideClearance that covers
// the bucket and prefix, and a clearance exists only when, in the same run,
// the bucket's lifecycle rules were read and none that hides or deletes
// versions reaches a key under the prefix (ReadB2HideClearance), or the
// operator stated so for a store whose rules cannot be read
// (StatedHideClearance). A failed or unparseable read of the rules is a
// refusal, never a clearance.

// ErrLifecycleRule is returned when a lifecycle rule hides or deletes
// versions of keys under the prefix a hide would cover.
var ErrLifecycleRule = errors.New("a lifecycle rule hides or deletes versions under the prefix")

// HideClearance is the finding that no lifecycle rule hides or deletes the
// versions of keys under one prefix of one bucket. The zero value clears
// nothing; only ReadB2HideClearance and StatedHideClearance make one.
type HideClearance struct {
	bucket string
	prefix string
	source string
}

// StatedHideClearance clears bucket and prefix on the operator's statement
// that the bucket has no lifecycle rule that hides or deletes versions under
// the prefix, for a store whose rules the program cannot read.
func StatedHideClearance(bucket, prefix string) HideClearance {
	return HideClearance{bucket: bucket, prefix: prefix, source: "the operator's statement"}
}

// Covers reports whether the clearance was made for bucket and prefix.
func (c HideClearance) Covers(bucket, prefix string) bool {
	return c.source != "" && c.bucket == bucket && c.prefix == prefix
}

// Source says what the clearance rests on, or "" for the zero value.
func (c HideClearance) Source() string {
	return c.source
}

// ReadB2HideClearance reads the lifecycle rules of bucket through B2's native
// API and clears prefix when no rule reaches it. Any failure to read or parse
// the rules is an error and no clearance.
func ReadB2HideClearance(ctx context.Context, account B2Account, bucket, prefix string) (HideClearance, error) {
	rules, err := ReadB2LifecycleRules(ctx, account, bucket)
	if err != nil {
		return HideClearance{}, errors.Wrap(err, "read the lifecycle rules of "+bucket)
	}
	for _, rule := range rules {
		if rule.Reaches(prefix) {
			return HideClearance{}, errors.Wrapf(ErrLifecycleRule, "bucket %s: the rule on prefix %q sets %s", bucket, rule.FileNamePrefix, strings.Join(rule.Expiring, ", "))
		}
	}
	return HideClearance{bucket: bucket, prefix: prefix, source: "the bucket's lifecycle rules, read from B2"}, nil
}

package bucket_setup

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// TestExecuteCompletesWithoutBuckets checks that a controller with no usable
// bucket configuration ends execution instead of waiting for cancellation.
func TestExecuteCompletesWithoutBuckets(t *testing.T) {
	// Configure one entry without a bucket config, which is skipped.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	c := NewController(logrus.NewEntry(logrus.New()), nil, &Config{
		ApplyBucketConfigs: []*ApplyBucketConfig{{}},
	})

	// Expect Execute to finish before the deadline.
	if err := c.Execute(ctx); err != nil {
		t.Fatalf("expected completion, got %v", err)
	}
}

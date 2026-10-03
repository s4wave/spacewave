package volume_controller

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/volume"
)

// TestApplyBucketConfigPublishesStorageError prevents failed creation from leaving callers waiting.
func TestApplyBucketConfigPublishesStorageError(t *testing.T) {
	// Create a volume whose bucket configuration fails in storage.
	ctx := t.Context()
	failure := errors.New("bucket storage failed")
	ctrl := &Controller{
		config: &Config{},
		volume: ccontainer.NewCContainer(&volumeCtxPair{
			vol: &failedBucketVolume{failure: failure},
			ctx: ctx,
		}),
	}

	// Resolve the bucket configuration request against the failing volume.
	resolver := &applyBucketConfigResolver{
		c:   ctrl,
		dir: bucket.NewApplyBucketConfigToVolume(&bucket.Config{Id: "space", Rev: 1}, "volume"),
	}
	handler := &bucketConfigResultHandler{}
	if err := resolver.Resolve(ctx, handler); err != nil {
		t.Fatal(err)
	}

	// Verify the resolver published one failure for the requested bucket.
	if len(handler.results) != 1 {
		t.Fatalf("published %d results, want one storage error", len(handler.results))
	}
	result := handler.results[0]
	if result.GetError() != failure.Error() || result.GetBucketId() != "space" || result.GetVolumeId() != "volume" {
		t.Fatalf("failure result=%v", result)
	}
	if result.GetUpdated() || result.GetBucketConf() != nil {
		t.Fatalf("failure published a successful update: %v", result)
	}
}

// failedBucketVolume fails creation before any bucket configuration exists.
type failedBucketVolume struct {
	volume.Volume
	failure error
}

// GetID returns the fixture volume identity.
func (v *failedBucketVolume) GetID() string { return "volume" }

// ApplyBucketConfig returns the configured storage failure.
func (v *failedBucketVolume) ApplyBucketConfig(context.Context, *bucket.Config) (bool, *bucket.Config, *bucket.Config, error) {
	return false, nil, nil, v.failure
}

// bucketConfigResultHandler captures the resolver's published results.
type bucketConfigResultHandler struct {
	directive.ResolverHandler
	results []*bucket.ApplyBucketConfigResult
}

// AddValue retains each published result.
func (h *bucketConfigResultHandler) AddValue(value directive.Value) (uint32, bool) {
	h.results = append(h.results, value.(*bucket.ApplyBucketConfigResult))
	return uint32(len(h.results)), true
}

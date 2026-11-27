package plugin_host_scheduler

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// bucketMountWake wakes a manifest selection loop when a bucket referenced by
// an unreadable startup candidate mounts. A returning visitor's World can link
// only manifests stored in a remote release bucket that mounts after the first
// selection reads them, and the mount changes nothing in the World.
type bucketMountWake struct {
	// ctx bounds every lookup. Canceling ctx releases the lookups.
	ctx context.Context
	// b resolves the bucket lookups.
	b bus.Bus
	// wake re-runs the selection loop.
	wake func()
	// watched holds the bucket IDs already looked up. Only the selection loop
	// calls watch, so the map needs no lock.
	watched map[string]struct{}
}

// newBucketMountWake constructs a bucketMountWake bound to ctx.
func newBucketMountWake(ctx context.Context, b bus.Bus, wake func()) *bucketMountWake {
	return &bucketMountWake{ctx: ctx, b: b, wake: wake, watched: make(map[string]struct{})}
}

// watch looks up, once per bucket, each unmounted bucket a candidate
// references, and wakes the loop when a lookup yields a mounted bucket.
func (w *bucketMountWake) watch(ctx context.Context, candidates []*bldr_manifest_world.StartupManifestCandidateEligibility) error {
	for _, candidate := range candidates {
		// Look up each unmounted bucket once.
		bucketID := candidate.UnmountedBucketID
		if bucketID == "" {
			continue
		}
		if _, ok := w.watched[bucketID]; ok {
			continue
		}
		w.watched[bucketID] = struct{}{}

		// The resolver adds a value only once the bucket config is known.
		_, ref, err := w.b.AddDirective(
			ctx,
			bucket_lookup.NewBuildBucketLookup(bucketID),
			directive.NewCallbackHandler(func(directive.AttachedValue) { w.wake() }, nil, nil),
		)
		if err != nil {
			return err
		}
		context.AfterFunc(w.ctx, ref.Release)
	}
	return nil
}

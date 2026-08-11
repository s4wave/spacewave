package cdn_bstore

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

const (
	releaseWorldManifestProbeEnv        = "SPACEWAVE_RELEASE_WORLD_MANIFEST_PROBE"
	releaseWorldManifestProbeBaseURLEnv = "SPACEWAVE_RELEASE_WORLD_CDN_BASE_URL"
	releaseWorldManifestProbeSpaceIDEnv = "SPACEWAVE_RELEASE_WORLD_CDN_SPACE_ID"

	// One initial tail per observed pack leaves a small payload allowance. This
	// is deliberately RED for the observed 177-range startup shape.
	releaseWorldColdRangeBudget uint64 = 64
)

var releaseWorldStartupManifestIDs = []string{
	"spacewave-core",
	"spacewave-web",
	"spacewave-app",
	"web",
}

// TestReleaseWorldManifestStartupRangeBudget reproduces the cold Release World
// startup read without building a release, staging app, or browser. It is
// opt-in because it reads the supplied public CDN Space.
func TestReleaseWorldManifestStartupRangeBudget(t *testing.T) {
	baseURL, spaceID := releaseWorldManifestProbeConfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store, err := NewCdnBlockStore(Options{
		CdnBaseURL: baseURL,
		SpaceID:    spaceID,
		PointerTTL: -1,
	})
	if err != nil {
		t.Fatalf("new CDN block store: %v", err)
	}
	t.Cleanup(store.Close)

	beforeEngine := store.pfs.SnapshotStats()
	engine, releaseEngine := newReleaseWorldProbeEngine(t, ctx, store)
	t.Cleanup(releaseEngine)
	beforeCold := store.pfs.SnapshotStats()

	coldPhases := newReleaseWorldProbePhases(store, beforeCold)
	coldManifests, coldManifestErrs := collectReleaseWorldStartupManifests(t, ctx, engine, coldPhases)
	coldPhases.finish()
	cold := store.pfs.SnapshotStats()
	coldStats := releaseWorldProbeStatsDelta(cold, beforeEngine)
	logReleaseWorldProbeStats(t, "engine", releaseWorldProbeStatsDelta(beforeCold, beforeEngine), beforeCold.EngineCount)
	coldPhases.log(t)
	logReleaseWorldProbeStats(t, "cold", coldStats, cold.EngineCount)
	logReleaseWorldManifestResult(t, "cold", coldManifests, coldManifestErrs)

	beforeWarm := cold
	warmManifests, warmManifestErrs := collectReleaseWorldStartupManifests(t, ctx, engine, nil)
	warm := store.pfs.SnapshotStats()
	warmStats := releaseWorldProbeStatsDelta(warm, beforeWarm)
	logReleaseWorldProbeStats(t, "warm", warmStats, warm.EngineCount)
	logReleaseWorldManifestResult(t, "warm", warmManifests, warmManifestErrs)

	if coldStats.rangeRequests > releaseWorldColdRangeBudget {
		t.Errorf(
			"cold Release World startup used %d Range requests, want at most %d",
			coldStats.rangeRequests,
			releaseWorldColdRangeBudget,
		)
	}
	if warmStats.rangeRequests != 0 {
		t.Errorf("warm Release World startup used %d Range requests, want 0", warmStats.rangeRequests)
	}
}

func releaseWorldManifestProbeConfig(t *testing.T) (string, string) {
	t.Helper()
	if os.Getenv(releaseWorldManifestProbeEnv) != "1" {
		t.Skipf(
			"set %s=1, %s, and %s to probe a public Release World",
			releaseWorldManifestProbeEnv,
			releaseWorldManifestProbeBaseURLEnv,
			releaseWorldManifestProbeSpaceIDEnv,
		)
	}
	baseURL := os.Getenv(releaseWorldManifestProbeBaseURLEnv)
	spaceID := os.Getenv(releaseWorldManifestProbeSpaceIDEnv)
	if baseURL == "" || spaceID == "" {
		t.Fatalf(
			"%s and %s are required when %s=1",
			releaseWorldManifestProbeBaseURLEnv,
			releaseWorldManifestProbeSpaceIDEnv,
			releaseWorldManifestProbeEnv,
		)
	}
	return baseURL, spaceID
}

func newReleaseWorldProbeEngine(
	t *testing.T,
	ctx context.Context,
	store *CdnBlockStore,
) (*world_block.Engine, func()) {
	t.Helper()
	if _, err := store.Refresh(ctx); err != nil {
		t.Fatalf("refresh CDN root pointer: %v", err)
	}
	pointer := store.Pointer()
	if pointer == nil || pointer.GetRoot() == nil {
		t.Fatal("CDN root pointer has no shared-object root")
	}

	soRootInner := &sobject.SORootInner{}
	if err := soRootInner.UnmarshalVT(pointer.GetRoot().GetInner()); err != nil {
		t.Fatalf("decode CDN shared-object root: %v", err)
	}
	inner := &sobject_world_engine.InnerState{}
	if err := inner.UnmarshalVT(soRootInner.GetStateData()); err != nil {
		t.Fatalf("decode CDN world head: %v", err)
	}
	if inner.GetHeadRef() == nil {
		t.Fatal("CDN shared object has no published world head")
	}

	logger := logrus.NewEntry(logrus.New())
	headRef := inner.GetHeadRef().CloneVT()
	bucketID := store.GetID()
	headRef.BucketId = bucketID
	transformConf := headRef.GetTransformConf()
	sfs := transform_all.BuildFactorySet()
	transformer := block_transform.NewTransformerWithSteps(nil)
	if len(transformConf.GetSteps()) != 0 {
		var err error
		transformer, err = block_transform.NewTransformer(
			controller.ConstructOpts{Logger: logger},
			sfs,
			transformConf,
		)
		if err != nil {
			t.Fatalf("build CDN world transformer: %v", err)
		}
	}

	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		logger,
		sfs,
		store,
		transformer,
		headRef,
		&bucket.BucketOpArgs{BucketId: bucketID, VolumeId: bucketID},
		transformConf,
	)
	cursor.SetBucketIDOverride(bucketID)
	cursor.SetDecodedBlockCache(store.GetDecodedBlockCache())
	engine, err := world_block.NewEngine(ctx, logger, cursor, nil, nil, false)
	if err != nil {
		cursor.Release()
		t.Fatalf("new CDN world engine: %v", err)
	}
	return engine, cursor.Release
}

func collectReleaseWorldStartupManifests(
	t *testing.T,
	ctx context.Context,
	engine *world_block.Engine,
	phases *releaseWorldProbePhases,
) (map[string][]*bldr_manifest_world.CollectedManifest, []error) {
	t.Helper()
	var manifests map[string][]*bldr_manifest_world.CollectedManifest
	var manifestErrs []error
	err := world.ExecTransaction(ctx, engine, false, func(ctx context.Context, ws world.WorldState) error {
		if phases != nil {
			ws = &releaseWorldProbeState{WorldState: ws, phases: phases}
		}
		var err error
		manifests, manifestErrs, err = bldr_manifest_world.CollectStartupManifestsForManifestIDs(
			ctx,
			ws,
			releaseWorldStartupManifestIDs,
			nil,
			"spacewave/release/manifests",
		)
		return err
	})
	if err != nil {
		t.Fatalf("collect Release World startup manifests: %v", err)
	}
	for _, manifestID := range releaseWorldStartupManifestIDs {
		if len(manifests[manifestID]) == 0 {
			t.Errorf("Release World startup collection omitted %q", manifestID)
		}
	}
	return manifests, manifestErrs
}

type releaseWorldProbeState struct {
	world.WorldState
	phases *releaseWorldProbePhases
}

func (s *releaseWorldProbeState) LookupGraphQuadsBatch(
	ctx context.Context,
	filters []world.GraphQuad,
	limitPerFilter uint32,
) ([][]world.GraphQuad, error) {
	s.phases.enter("candidate-graph")
	before := s.phases.store.pfs.SnapshotStats()
	results, err := s.WorldState.LookupGraphQuadsBatch(ctx, filters, limitPerFilter)
	s.phases.recordGraphBatch(filters, results, before, s.phases.store.pfs.SnapshotStats())
	return results, err
}

func (s *releaseWorldProbeState) LookupGraphQuads(
	ctx context.Context,
	filter world.GraphQuad,
	limit uint32,
) ([]world.GraphQuad, error) {
	s.phases.enter("metadata-type")
	return s.WorldState.LookupGraphQuads(ctx, filter, limit)
}

func (s *releaseWorldProbeState) GetObject(
	ctx context.Context,
	key string,
) (world.ObjectState, bool, error) {
	s.phases.enter("metadata-type")
	obj, found, err := s.WorldState.GetObject(ctx, key)
	if !found || obj == nil {
		return obj, found, err
	}
	return &releaseWorldProbeObject{ObjectState: obj, phases: s.phases}, found, err
}

func (s *releaseWorldProbeState) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	s.phases.enter("manifest-body")
	return s.WorldState.BuildStorageCursor(ctx)
}

type releaseWorldProbeObject struct {
	world.ObjectState
	phases *releaseWorldProbePhases
}

func (o *releaseWorldProbeObject) GetRootRef(ctx context.Context) (*bucket.ObjectRef, uint64, error) {
	o.phases.enter("metadata-type")
	return o.ObjectState.GetRootRef(ctx)
}

func (o *releaseWorldProbeObject) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	o.phases.enter("manifest-body")
	return o.ObjectState.AccessWorldState(ctx, ref, cb)
}

type releaseWorldProbePhases struct {
	store        *CdnBlockStore
	current      string
	last         packfile_store.PackfileStoreStats
	stats        map[string]releaseWorldProbeStats
	graphBatches []releaseWorldProbeGraphBatch
}

func newReleaseWorldProbePhases(
	store *CdnBlockStore,
	initial packfile_store.PackfileStoreStats,
) *releaseWorldProbePhases {
	return &releaseWorldProbePhases{
		store:   store,
		current: "metadata-type",
		last:    initial,
		stats:   make(map[string]releaseWorldProbeStats),
	}
}

func (p *releaseWorldProbePhases) enter(phase string) {
	if p.current == phase {
		return
	}
	next := p.store.pfs.SnapshotStats()
	p.stats[p.current] = p.stats[p.current].add(releaseWorldProbeStatsDelta(next, p.last))
	p.last = next
	p.current = phase
}

func (p *releaseWorldProbePhases) finish() {
	next := p.store.pfs.SnapshotStats()
	p.stats[p.current] = p.stats[p.current].add(releaseWorldProbeStatsDelta(next, p.last))
	p.last = next
}

func (p *releaseWorldProbePhases) log(t *testing.T) {
	t.Helper()
	for _, phase := range []string{"candidate-graph", "metadata-type", "manifest-body"} {
		logReleaseWorldProbeStats(t, "cold "+phase, p.stats[phase], 0)
	}
	for i, batch := range p.graphBatches {
		logReleaseWorldProbeGraphBatch(t, i+1, batch)
	}
}

type releaseWorldProbeGraphBatch struct {
	filterCount int
	subjects    []string
	labels      []string
	quadCount   int
	stats       releaseWorldProbeStats
}

func (p *releaseWorldProbePhases) recordGraphBatch(
	filters []world.GraphQuad,
	results [][]world.GraphQuad,
	before packfile_store.PackfileStoreStats,
	after packfile_store.PackfileStoreStats,
) {
	batch := releaseWorldProbeGraphBatch{
		filterCount: len(filters),
		stats:       releaseWorldProbeStatsDelta(after, before),
	}
	for _, filter := range filters {
		if subject := filter.GetSubject(); !slices.Contains(batch.subjects, subject) {
			batch.subjects = append(batch.subjects, subject)
		}
		if label := filter.GetLabel(); !slices.Contains(batch.labels, label) {
			batch.labels = append(batch.labels, label)
		}
	}
	for _, quads := range results {
		batch.quadCount += len(quads)
	}
	p.graphBatches = append(p.graphBatches, batch)
}

type releaseWorldProbeStats struct {
	lookups        uint64
	candidates     uint64
	openedPacks    uint64
	negativePacks  uint64
	targetHits     uint64
	rangeRequests  uint64
	indexTailFetch uint64
}

func releaseWorldProbeStatsDelta(
	after packfile_store.PackfileStoreStats,
	before packfile_store.PackfileStoreStats,
) releaseWorldProbeStats {
	return releaseWorldProbeStats{
		lookups:        after.LookupCount - before.LookupCount,
		candidates:     after.CandidatePacks - before.CandidatePacks,
		openedPacks:    after.OpenedPacks - before.OpenedPacks,
		negativePacks:  after.NegativePacks - before.NegativePacks,
		targetHits:     after.TargetHits - before.TargetHits,
		rangeRequests:  after.RangeRequestCount - before.RangeRequestCount,
		indexTailFetch: after.IndexTailFetchCount - before.IndexTailFetchCount,
	}
}

func (s releaseWorldProbeStats) add(other releaseWorldProbeStats) releaseWorldProbeStats {
	s.lookups += other.lookups
	s.candidates += other.candidates
	s.openedPacks += other.openedPacks
	s.negativePacks += other.negativePacks
	s.targetHits += other.targetHits
	s.rangeRequests += other.rangeRequests
	s.indexTailFetch += other.indexTailFetch
	return s
}

func logReleaseWorldProbeStats(
	t *testing.T,
	phase string,
	stats releaseWorldProbeStats,
	engines int,
) {
	t.Helper()
	t.Logf(
		"%s lookups=%d bloom-candidates=%d opened-packs=%d negative-packs=%d hits=%d engines=%d ranges=%d index-tails=%d",
		phase,
		stats.lookups,
		stats.candidates,
		stats.openedPacks,
		stats.negativePacks,
		stats.targetHits,
		engines,
		stats.rangeRequests,
		stats.indexTailFetch,
	)
}

func logReleaseWorldProbeGraphBatch(t *testing.T, index int, batch releaseWorldProbeGraphBatch) {
	t.Helper()
	t.Logf(
		"cold candidate-graph call=%d filters=%d subjects=%q labels=%q returned-quads=%d lookups=%d bloom-candidates=%d opened-packs=%d negative-packs=%d hits=%d ranges=%d index-tails=%d",
		index,
		batch.filterCount,
		batch.subjects,
		batch.labels,
		batch.quadCount,
		batch.stats.lookups,
		batch.stats.candidates,
		batch.stats.openedPacks,
		batch.stats.negativePacks,
		batch.stats.targetHits,
		batch.stats.rangeRequests,
		batch.stats.indexTailFetch,
	)
}

func logReleaseWorldManifestResult(
	t *testing.T,
	phase string,
	manifests map[string][]*bldr_manifest_world.CollectedManifest,
	manifestErrs []error,
) {
	t.Helper()
	t.Logf("%s collected manifest-ids=%d skipped-candidates=%d", phase, len(manifests), len(manifestErrs))
	for _, err := range manifestErrs {
		t.Logf("%s skipped candidate: %v", phase, err)
	}
}

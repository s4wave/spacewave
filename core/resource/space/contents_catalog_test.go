package resource_space

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

type catalogCacheTestManifestSpec struct {
	key         string
	pluginID    string
	description string
	revision    uint64
}

func TestSpaceContentsResourceWatchStateCachesAvailablePluginCatalogForUnchangedManifestSet(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	manifestSpecs := catalogCacheTestManifestSpecs()
	manifests := seedCatalogCacheTestManifests(t, ctx, tb.Engine, manifestSpecs)

	resource := NewSpaceContentsResource(
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.Engine,
		"space-test",
		tb.EngineID,
	)
	resource.volumeID = tb.EngineVolumeID
	resource.storeID = "platform-account"
	defer resource.Release()

	var lookupCalls atomic.Int64
	resource.lookupManifest = func(
		_ context.Context,
		_ world.WorldState,
		key string,
	) (*bldr_manifest.Manifest, *bucket.ObjectRef, error) {
		lookupCalls.Add(1)
		return manifests[key], nil, nil
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	stream := newTestWatchSpaceContentsStateStream(watchCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- resource.WatchState(&s4wave_space.WatchSpaceContentsStateRequest{}, stream)
	}()

	initial := receiveTestSpaceContentsState(t, ctx, stream)
	if got := int(lookupCalls.Load()); got != len(manifestSpecs) {
		watchCancel()
		t.Fatalf("initial emission looked up %d manifests, want %d", got, len(manifestSpecs))
	}
	assertCatalogCacheTestAvailablePlugins(t, initial.GetAvailablePlugins(), manifestSpecs)

	settingsTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		watchCancel()
		t.Fatalf("NewTransaction(write settings): %v", err)
	}
	defer settingsTx.Discard()
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		settingsTx,
		"",
		space_world_ops.DefaultSpaceSettingsObjectKey,
		&space_world.SpaceSettings{},
		true,
		time.Now(),
	); err != nil {
		watchCancel()
		t.Fatalf("SetSpaceSettings: %v", err)
	}
	if err := settingsTx.Commit(ctx); err != nil {
		watchCancel()
		t.Fatalf("Commit(write settings): %v", err)
	}

	next := receiveTestSpaceContentsState(t, ctx, stream)
	if got := int(lookupCalls.Load()); got != len(manifestSpecs) {
		watchCancel()
		t.Fatalf("unchanged manifest set performed %d total lookups after unrelated write, want %d", got, len(manifestSpecs))
	}
	assertCatalogCacheTestAvailablePlugins(t, next.GetAvailablePlugins(), manifestSpecs)

	watchCancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("WatchState: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for WatchState to stop")
	}
}

func TestSpaceContentsResourceWatchStateInvalidatesAvailablePluginCatalogWhenManifestRootRefChanges(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	manifestSpecs := catalogCacheTestManifestSpecs()
	manifests := seedCatalogCacheTestManifests(t, ctx, tb.Engine, manifestSpecs)

	resource := NewSpaceContentsResource(
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.Engine,
		"space-test",
		tb.EngineID,
	)
	resource.volumeID = tb.EngineVolumeID
	resource.storeID = "platform-account"
	defer resource.Release()

	var lookupCalls atomic.Int64
	resource.lookupManifest = func(
		_ context.Context,
		_ world.WorldState,
		key string,
	) (*bldr_manifest.Manifest, *bucket.ObjectRef, error) {
		lookupCalls.Add(1)
		return manifests[key], nil, nil
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	stream := newTestWatchSpaceContentsStateStream(watchCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- resource.WatchState(&s4wave_space.WatchSpaceContentsStateRequest{}, stream)
	}()

	initial := receiveTestSpaceContentsState(t, ctx, stream)
	if got := int(lookupCalls.Load()); got != len(manifestSpecs) {
		watchCancel()
		t.Fatalf("initial emission looked up %d manifests, want %d", got, len(manifestSpecs))
	}
	assertCatalogCacheTestAvailablePlugins(t, initial.GetAvailablePlugins(), manifestSpecs)

	updatedSpec := manifestSpecs[1]
	updatedSpec.description = "beta catalog entry after root ref update"
	updatedSpec.revision = 12
	manifestSpecs[1] = updatedSpec
	manifests[updatedSpec.key] = catalogCacheTestManifest(updatedSpec)

	rootRefTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		watchCancel()
		t.Fatalf("NewTransaction(update manifest root ref): %v", err)
	}
	defer rootRefTx.Discard()
	manifestObject, err := world.MustGetObject(ctx, rootRefTx, updatedSpec.key)
	if err != nil {
		watchCancel()
		t.Fatalf("MustGetObject(%s): %v", updatedSpec.key, err)
	}
	if _, err := manifestObject.SetRootRef(ctx, catalogCacheTestObjectRef(t, updatedSpec.key+"/updated-root")); err != nil {
		watchCancel()
		t.Fatalf("SetRootRef(%s): %v", updatedSpec.key, err)
	}
	if err := rootRefTx.Commit(ctx); err != nil {
		watchCancel()
		t.Fatalf("Commit(update manifest root ref): %v", err)
	}

	next := receiveTestSpaceContentsState(t, ctx, stream)
	expectedLookups := len(manifestSpecs) * 2
	if got := int(lookupCalls.Load()); got != expectedLookups {
		watchCancel()
		t.Fatalf("root-ref change performed %d total lookups, want %d", got, expectedLookups)
	}
	assertCatalogCacheTestAvailablePlugins(t, next.GetAvailablePlugins(), manifestSpecs)

	watchCancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("WatchState: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for WatchState to stop")
	}
}

func catalogCacheTestManifestSpecs() []catalogCacheTestManifestSpec {
	return []catalogCacheTestManifestSpec{
		{
			key:         "catalog-cache/manifest-alpha",
			pluginID:    "catalog-cache-alpha",
			description: "alpha catalog entry",
			revision:    1,
		},
		{
			key:         "catalog-cache/manifest-beta",
			pluginID:    "catalog-cache-beta",
			description: "beta catalog entry",
			revision:    2,
		},
		{
			key:         "catalog-cache/manifest-gamma",
			pluginID:    "catalog-cache-gamma",
			description: "gamma catalog entry",
			revision:    3,
		},
	}
}

func seedCatalogCacheTestManifests(
	t *testing.T,
	ctx context.Context,
	engine world.Engine,
	specs []catalogCacheTestManifestSpec,
) map[string]*bldr_manifest.Manifest {
	t.Helper()

	manifests := make(map[string]*bldr_manifest.Manifest, len(specs))
	seedTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(seed manifests): %v", err)
	}
	defer seedTx.Discard()
	for _, spec := range specs {
		manifests[spec.key] = catalogCacheTestManifest(spec)
		if _, err := seedTx.CreateObject(ctx, spec.key, catalogCacheTestObjectRef(t, spec.key+"/initial-root")); err != nil {
			t.Fatalf("CreateObject(%s): %v", spec.key, err)
		}
		if err := world_types.SetObjectType(ctx, seedTx, spec.key, bldr_manifest_world.ManifestTypeID); err != nil {
			t.Fatalf("SetObjectType(%s): %v", spec.key, err)
		}
	}
	if err := seedTx.Commit(ctx); err != nil {
		t.Fatalf("Commit(seed manifests): %v", err)
	}
	return manifests
}

func catalogCacheTestManifest(spec catalogCacheTestManifestSpec) *bldr_manifest.Manifest {
	meta := bldr_manifest.NewManifestMeta(spec.pluginID, bldr_manifest.BuildType_DEV, "web/js", spec.revision)
	meta.Description = spec.description
	return bldr_manifest.NewManifest(meta, "entrypoint.js")
}

func catalogCacheTestObjectRef(t *testing.T, seed string) *bucket.ObjectRef {
	t.Helper()
	rootRef, err := block.BuildBlockRef([]byte(seed), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef(%q): %v", seed, err)
	}
	return &bucket.ObjectRef{RootRef: rootRef}
}

func receiveTestSpaceContentsState(
	t *testing.T,
	ctx context.Context,
	stream *testWatchSpaceContentsStateStream,
) *s4wave_space.SpaceContentsState {
	t.Helper()
	select {
	case resp := <-stream.msgs:
		return resp
	case <-ctx.Done():
		t.Fatal("timed out waiting for space contents state")
		return nil
	}
}

func assertCatalogCacheTestAvailablePlugins(
	t *testing.T,
	got []*s4wave_space.AvailablePlugin,
	specs []catalogCacheTestManifestSpec,
) {
	t.Helper()
	if len(got) != len(specs) {
		t.Fatalf("available plugin count = %d, want %d: %+v", len(got), len(specs), got)
	}
	for i, spec := range specs {
		if got[i].GetPluginId() != spec.pluginID {
			t.Fatalf("available plugin %d id = %q, want %q", i, got[i].GetPluginId(), spec.pluginID)
		}
		if got[i].GetDescription() != spec.description {
			t.Fatalf("available plugin %s description = %q, want %q", spec.pluginID, got[i].GetDescription(), spec.description)
		}
		if got[i].GetRevision() != strconv.FormatUint(spec.revision, 10) {
			t.Fatalf("available plugin %s revision = %q, want %d", spec.pluginID, got[i].GetRevision(), spec.revision)
		}
	}
}

func TestAvailablePluginsFromCatalogKeepsHighestRev(t *testing.T) {
	catalog := map[string]*bldr_manifest.ManifestMeta{}
	// Two revisions of the same plugin plus a distinct plugin; the catalog must
	// keep the highest revision per manifest ID.
	for _, meta := range []*bldr_manifest.ManifestMeta{
		{ManifestId: "spacewave-notes", Rev: 2, Description: "notes v2"},
		{ManifestId: "spacewave-notes", Rev: 5, Description: "notes v5"},
		{ManifestId: "spacewave-notes", Rev: 1, Description: "notes v1"},
		{ManifestId: "spacewave-v86", Rev: 3, Description: "vm"},
		{ManifestId: "", Rev: 9, Description: "unnamed"},
	} {
		addManifestToCatalog(catalog, meta)
	}

	got := availablePluginsFromCatalog(catalog)
	if len(got) != 2 {
		t.Fatalf("expected 2 plugins, got %d: %+v", len(got), got)
	}

	// Sorted by plugin ID: notes before v86.
	if got[0].GetPluginId() != "spacewave-notes" || got[1].GetPluginId() != "spacewave-v86" {
		t.Fatalf("unexpected sort order: %s, %s", got[0].GetPluginId(), got[1].GetPluginId())
	}
	if got[0].GetRevision() != "5" {
		t.Fatalf("expected highest revision 5, got %q", got[0].GetRevision())
	}
	if got[0].GetDescription() != "notes v5" {
		t.Fatalf("expected highest-rev description, got %q", got[0].GetDescription())
	}
	if got[1].GetRevision() != "3" {
		t.Fatalf("expected revision 3 for v86, got %q", got[1].GetRevision())
	}
}

func TestAvailablePluginsFromCatalogEmpty(t *testing.T) {
	got := availablePluginsFromCatalog(map[string]*bldr_manifest.ManifestMeta{})
	if len(got) != 0 {
		t.Fatalf("expected empty catalog, got %+v", got)
	}
}

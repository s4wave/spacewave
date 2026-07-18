package resource_space

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/s4wave/spacewave/testbed"
)

const (
	pluginReadinessPluginID   = "test-readiness-plugin"
	pluginReadinessPlatformID = "test/platform"
	pluginReadinessTypeID     = "test/readiness"
)

func TestSpaceResourceWaitsForDesiredPluginTypeRegistration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(plugin_space.NewFactory(tb.Bus))

	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		"",
		space_world_ops.DefaultSpaceSettingsObjectKey,
		&space_world.SpaceSettings{PluginIds: []string{pluginReadinessPluginID}},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	storeKey := "test/plugin-manifests"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, tb.WorldState, storeKey); err != nil {
		t.Fatal(err)
	}
	manifestRef := createPluginReadinessManifest(t, ctx, tb)
	const manifestKey = "test/manifests/readiness"
	if err := bldr_manifest_world.ExStoreManifestOp(
		ctx,
		tb.WorldState,
		tb.Volume.GetPeerID(),
		manifestKey,
		[]string{storeKey},
		manifestRef,
	); err != nil {
		t.Fatal(err)
	}
	if err := tb.WorldState.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(
		manifestKey,
		manifestKey,
		pluginReadinessPluginID,
	)); err != nil {
		t.Fatal(err)
	}
	manifestKeys, err := world_types.ListObjectsWithType(ctx, tb.WorldState, bldr_manifest_world.ManifestTypeID)
	if err != nil {
		t.Fatal(err)
	}
	manifests, manifestErrs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		tb.WorldState,
		pluginReadinessPluginID,
		[]string{pluginReadinessPlatformID},
		manifestKeys...,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifestErrs) != 0 || len(manifests) != 1 {
		t.Fatalf(
			"valid imported Manifest resolved manifests=%d errors=%v keys=%v",
			len(manifests),
			manifestErrs,
			manifestKeys,
		)
	}

	loader := newPluginReadinessLoadController(tb.Bus)
	loaderRef, err := tb.Bus.AddController(ctx, loader, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer loaderRef()

	body := &spaceResourceChatBody{
		engine:   tb.BusEngine,
		engineID: tb.EngineID,
		bucketID: tb.EngineBucketID,
	}
	spaceResource := NewSpaceResource(tb.Logger, tb.Bus, body)
	resources := newSpaceRecordingResourceClient(ctx)
	mountCtx := resource_server.WithResourceClientContext(ctx, resources)
	if _, err := spaceResource.MountSpaceContents(
		mountCtx,
		&s4wave_space.MountSpaceContentsRequest{},
	); err != nil {
		t.Fatal(err)
	}

	select {
	case <-loader.loadStarted:
	case <-ctx.Done():
		t.Fatal("Space settings did not start the desired plugin")
	}

	select {
	case <-loader.manifestResolved:
	case <-ctx.Done():
		t.Fatal("desired plugin did not resolve the imported Manifest")
	}

	type lookupResult struct {
		objectType objecttype.ObjectType
		ref        directive.Reference
		err        error
	}
	lookupCtx := objecttype.WithEngineID(ctx, tb.EngineID)
	lookupResultCh := make(chan lookupResult, 1)
	go func() {
		ot, ref, err := objecttype.ExLookupObjectType(lookupCtx, tb.Bus, pluginReadinessTypeID)
		lookupResultCh <- lookupResult{objectType: ot, ref: ref, err: err}
	}()

	select {
	case result := <-lookupResultCh:
		if result.ref != nil {
			result.ref.Release()
		}
		t.Fatalf("typed lookup completed before registration: objectType=%v err=%v", result.objectType, result.err)
	case <-loader.lookupObserved:
	}

	close(loader.allowRegistration)
	var result lookupResult
	select {
	case result = <-lookupResultCh:
	case <-ctx.Done():
		t.Fatal("typed lookup did not resolve after registration")
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.objectType == nil || result.objectType.GetObjectTypeID() != pluginReadinessTypeID {
		t.Fatalf("typed lookup returned %v", result.objectType)
	}
	if result.ref == nil {
		t.Fatal("typed lookup returned no reference")
	}
	result.ref.Release()
	unknown, unknownRef, err := objecttype.ExLookupObjectType(
		lookupCtx,
		tb.Bus,
		"test/unknown-readiness",
	)
	if unknownRef != nil {
		unknownRef.Release()
	}
	if err != nil {
		t.Fatal(err)
	}
	if unknown != nil {
		t.Fatalf("unknown typed lookup returned %v", unknown)
	}

}

func createPluginReadinessManifest(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
) *bldr_manifest.ManifestRef {
	t.Helper()
	meta := bldr_manifest.NewManifestMeta(
		pluginReadinessPluginID,
		bldr_manifest.BuildType_DEV,
		pluginReadinessPlatformID,
		1,
	)
	var manifestRef *bldr_manifest.ManifestRef
	if err := tb.Engine.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		transaction, blocks := cursor.BuildTransactionAtRef(nil, nil)
		blocks.SetBlock(bldr_manifest.NewManifest(meta, "entrypoint"), true)
		rootRef, _, err := transaction.Write(ctx, true)
		if err != nil {
			return err
		}
		objectRef := cursor.GetRef().CloneVT()
		objectRef.RootRef = rootRef
		manifestRef = bldr_manifest.NewManifestRef(meta, objectRef)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return manifestRef
}

type pluginReadinessLoadController struct {
	b                 bus.Bus
	loadStarted       chan struct{}
	manifestResolved  chan struct{}
	lookupObserved    chan struct{}
	allowRegistration chan struct{}
	lookupOnce        sync.Once
}

func newPluginReadinessLoadController(b bus.Bus) *pluginReadinessLoadController {
	return &pluginReadinessLoadController{
		b:                 b,
		loadStarted:       make(chan struct{}),
		manifestResolved:  make(chan struct{}),
		lookupObserved:    make(chan struct{}),
		allowRegistration: make(chan struct{}),
	}
}

func (c *pluginReadinessLoadController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/plugin-readiness-loader",
		controller.MustParseVersion("0.0.1"),
		"loads a delayed test plugin",
	)
}

func (c *pluginReadinessLoadController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (c *pluginReadinessLoadController) Close() error {
	return nil
}

func (c *pluginReadinessLoadController) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	if _, ok := inst.GetDirective().(objecttype.LookupObjectType); ok {
		c.lookupOnce.Do(func() {
			close(c.lookupObserved)
		})
		return nil, nil
	}
	load, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok || load.LoadPluginID() != pluginReadinessPluginID {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		close(c.loadStarted)
		manifestValue, _, manifestRef, err := bus.ExecWaitValue[*bldr_manifest.FetchManifestValue](
			ctx,
			c.b,
			bldr_manifest.NewFetchManifest(
				pluginReadinessPluginID,
				nil,
				[]string{pluginReadinessPlatformID},
				0,
			),
			func(_ bool, errs []error) (bool, error) {
				if len(errs) != 0 {
					return false, errs[0]
				}
				return true, nil
			},
			nil,
			nil,
		)
		if manifestRef != nil {
			defer manifestRef.Release()
		}
		if err != nil {
			return err
		}
		if len(manifestValue.GetManifestRefs()) == 0 {
			return nil
		}
		close(c.manifestResolved)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.allowRegistration:
		}
		registered := objecttype.NewObjectType(pluginReadinessTypeID, nil)
		objectTypeCtrl := objecttype_controller.NewController(func(
			_ context.Context,
			typeID string,
		) (objecttype.ObjectType, error) {
			if typeID != pluginReadinessTypeID {
				return nil, nil
			}
			return registered, nil
		})
		objectTypeRef, err := c.b.AddController(ctx, objectTypeCtrl, nil)
		if err != nil {
			return err
		}
		defer objectTypeRef()

		_, _ = handler.AddValue(bldr_plugin.NewRunningPlugin(nil))
		handler.MarkIdle(true)
		<-ctx.Done()
		return ctx.Err()
	}), nil)
}

var _ controller.Controller = (*pluginReadinessLoadController)(nil)

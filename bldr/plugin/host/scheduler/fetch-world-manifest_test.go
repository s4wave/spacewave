package plugin_host_scheduler

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/routine"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/sirupsen/logrus"
)

func TestDirectFetchHandlerPreservesCurrentStateAcrossEmptyGap(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	host1 := &testPluginHost{id: "desktop/linux/amd64"}
	host2 := &testPluginHost{id: "desktop/linux/amd64"}
	pi := &pluginInstance{
		c: &Controller{
			conf: &Config{},
		},
		le:                      le,
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	handler := pi.newDirectFetchHandler(&pluginHostSet{pluginHosts: []bldr_plugin_host.PluginHost{host1}})

	val1 := bldr_manifest.NewFetchManifestValue([]*bldr_manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 1, "bucket-1"),
	})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(1, val1))

	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != host1 || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state to be set from first fetched manifest")
	}
	if execState.manifestSnapshot.GetManifestRef() == nil {
		t.Fatal("expected manifest snapshot ref to be set")
	}

	handler.HandleValueRemoved(nil, directive.NewAttachedValue(1, val1))

	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != host1 {
		t.Fatal("expected execute state to remain during empty fetch-manifest gap")
	}
	if pi.downloadManifestRoutine.GetState() == nil {
		t.Fatal("expected download manifest state to remain during empty fetch-manifest gap")
	}
	originalExecState := execState

	handler.HandleValueAdded(nil, directive.NewAttachedValue(1, val1))

	execState = pi.executePluginRoutine.GetState()
	if execState != originalExecState {
		t.Fatal("expected re-adding the same manifest target to avoid resetting execute state")
	}

	val2 := bldr_manifest.NewFetchManifestValue([]*bldr_manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 2, "bucket-2"),
	})
	handler = pi.newDirectFetchHandler(&pluginHostSet{pluginHosts: []bldr_plugin_host.PluginHost{host2}})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(2, val2))

	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != host2 {
		t.Fatal("expected execute state to update to replacement plugin host")
	}
	if execState.manifestSnapshot.GetManifestRef() == nil {
		t.Fatal("expected replacement manifest snapshot ref to be set")
	}
}

func TestDirectFetchHandlerPrefersCurrentStateAcrossEqualRevOverlap(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	host := &testPluginHost{id: "desktop/linux/amd64"}
	pi := &pluginInstance{
		c: &Controller{
			conf: &Config{},
		},
		le:                      le,
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	handler := pi.newDirectFetchHandler(&pluginHostSet{pluginHosts: []bldr_plugin_host.PluginHost{host}})

	val1 := bldr_manifest.NewFetchManifestValue([]*bldr_manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 7, "bucket-a"),
	})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(1, val1))

	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state after first manifest")
	}
	firstRef := execState.manifestSnapshot.GetManifestRef()
	if firstRef == nil {
		t.Fatal("expected first manifest ref")
	}

	val2 := bldr_manifest.NewFetchManifestValue([]*bldr_manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 7, "bucket-b"),
	})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(2, val2))

	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state during equal-rev overlap")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(firstRef) {
		t.Fatal("expected equal-rev overlap to preserve the current execute target")
	}

	handler.HandleValueRemoved(nil, directive.NewAttachedValue(1, val1))

	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state after removing original candidate")
	}
	if execState.manifestSnapshot.GetManifestRef().EqualVT(firstRef) {
		t.Fatal("expected execute target to switch once the original candidate is removed")
	}
}

func newTestManifestRef(manifestID, platformID string, rev uint64, bucketID string) *bldr_manifest.ManifestRef {
	return bldr_manifest.NewManifestRef(
		bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_DEV, platformID, rev),
		&bucket.ObjectRef{BucketId: bucketID},
	)
}

type testPluginHost struct {
	id string
}

func (h *testPluginHost) GetPlatformId() string {
	return h.id
}

func (h *testPluginHost) Execute(ctx context.Context) error {
	return nil
}

func (h *testPluginHost) ListPlugins(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (h *testPluginHost) ExecutePlugin(
	ctx context.Context,
	pluginID,
	instanceKey,
	entrypoint string,
	pluginDist *unixfs.FSHandle,
	pluginAssets *unixfs.FSHandle,
	hostRpcMux srpc.Mux,
	rpcInit bldr_plugin_host.PluginRpcInitCb,
) error {
	return nil
}

func (h *testPluginHost) DeletePlugin(ctx context.Context, pluginID string) error {
	return nil
}

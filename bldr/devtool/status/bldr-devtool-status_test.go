//go:build !js

package status

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	cb_controller "github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	bldr_web_view_observer "github.com/s4wave/spacewave/bldr/web/view/observer"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/sirupsen/logrus"
)

func TestBldrDevtoolStatusClonesRows(t *testing.T) {
	fetchRows := []BldrDevtoolManifestFetchRow{{
		ID:         "fetch:web",
		ManifestID: "web",
		State:      BldrDevtoolManifestStateRunning,
	}}
	pluginRows := []BldrDevtoolPluginRow{{
		ID:       "plugin:web",
		PluginID: "web",
		State:    BldrDevtoolPluginStateRequested,
	}}

	snapshot := NewBldrDevtoolStatus(
		BldrDevtoolCommandStatus{Name: "start", State: BldrDevtoolCommandStateRunning},
		fetchRows,
		nil,
		pluginRows,
		nil,
		nil,
	)

	fetchRows[0].State = BldrDevtoolManifestStateError
	pluginRows[0].State = BldrDevtoolPluginStateErrored

	gotFetchRows := snapshot.GetManifestFetchRows()
	if gotFetchRows[0].State != BldrDevtoolManifestStateRunning {
		t.Fatalf("expected immutable fetch row, got %s", gotFetchRows[0].State)
	}
	gotPluginRows := snapshot.GetPluginRows()
	if gotPluginRows[0].State != BldrDevtoolPluginStateRequested {
		t.Fatalf("expected immutable plugin row, got %s", gotPluginRows[0].State)
	}

	gotFetchRows[0].State = BldrDevtoolManifestStateReady
	if snapshot.GetManifestFetchRows()[0].State != BldrDevtoolManifestStateRunning {
		t.Fatal("expected returned fetch rows to be cloned")
	}
}

func TestBldrDevtoolStatusWithRowsReturnsNewSnapshot(t *testing.T) {
	initial := EmptyBldrDevtoolStatus()
	next := initial.
		WithCommand(BldrDevtoolCommandStatus{Name: "build", State: BldrDevtoolCommandStateStarting}).
		WithManifestBuildRows([]BldrDevtoolManifestBuildRow{{
			ID:         "build:web",
			ManifestID: "web",
			State:      BldrDevtoolManifestStateQueued,
		}}).
		WithAttentionRows([]BldrDevtoolAttentionRow{{
			ID:       "err:1",
			Message:  "compile failed",
			Severity: BldrDevtoolAttentionSeverityError,
		}})

	if initial.GetCommand().Name != "" {
		t.Fatal("expected initial snapshot to remain unchanged")
	}
	if len(initial.GetManifestBuildRows()) != 0 {
		t.Fatal("expected initial build rows to remain unchanged")
	}
	if next.GetCommand().Name != "build" {
		t.Fatalf("expected copied command, got %q", next.GetCommand().Name)
	}
	if len(next.GetManifestBuildRows()) != 1 || len(next.GetAttentionRows()) != 1 {
		t.Fatal("expected copied row sets")
	}
}

func TestBldrDevtoolStatusProducerPublishesSnapshots(t *testing.T) {
	producer := NewBldrDevtoolStatusProducer(nil)
	initial := producer.GetStatus()

	producer.SetStatus(initial.WithCommand(BldrDevtoolCommandStatus{
		Name:  "start",
		State: BldrDevtoolCommandStateRunning,
	}))

	changed, err := producer.GetStatusCtr().WaitValueChange(context.Background(), initial, nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed.GetCommand().Name != "start" {
		t.Fatalf("expected start command, got %q", changed.GetCommand().Name)
	}

	source := changed.WithPluginRows([]BldrDevtoolPluginRow{{
		ID:       "plugin:desktop",
		PluginID: "desktop",
		State:    BldrDevtoolPluginStateRunning,
	}})
	producer.SetStatus(source)
	sourceRows := source.GetPluginRows()
	sourceRows[0].State = BldrDevtoolPluginStateErrored

	publishedRows := producer.GetStatus().GetPluginRows()
	if publishedRows[0].State != BldrDevtoolPluginStateRunning {
		t.Fatalf("expected producer to clone snapshots, got %s", publishedRows[0].State)
	}
}

func TestBldrDevtoolStatusProducerUpdateStatus(t *testing.T) {
	producer := NewBldrDevtoolStatusProducer(EmptyBldrDevtoolStatus())

	updated := producer.UpdateStatus(func(current *BldrDevtoolStatus) *BldrDevtoolStatus {
		return current.WithManifestFetchRows([]BldrDevtoolManifestFetchRow{{
			ID:         "fetch:browser",
			ManifestID: "browser",
			State:      BldrDevtoolManifestStateReady,
		}})
	})

	if len(updated.GetManifestFetchRows()) != 1 {
		t.Fatal("expected updated fetch rows")
	}
	if len(producer.GetStatus().GetManifestFetchRows()) != 1 {
		t.Fatal("expected producer status to update")
	}
}

func TestManifestBuildStatusAdapterPublishesLifecycleFields(t *testing.T) {
	producer := NewBldrDevtoolStatusProducer(nil)
	adapter := &manifestBuildStatusAdapter{producer: producer}

	adapter.SetManifestBuilderStatus(bldr_project_controller.ManifestBuilderStatus{
		ID:                      "build:web",
		BuildTargetIDs:          []string{"desktop"},
		ManifestID:              "web",
		PlatformID:              "browser",
		TargetPlatformIDs:       []string{"browser", "desktop/darwin/arm64"},
		BuildType:               "dev",
		RemoteID:                "devtool",
		State:                   bldr_project_controller.ManifestBuilderStatusStateRunning,
		FullRebuild:             true,
		WatchedFileCount:        3,
		DependencyRebuildReason: "manifest dependency changed: core",
		Summary:                 "full rebuild",
	})

	rows := producer.GetStatus().GetManifestBuildRows()
	if len(rows) != 1 {
		t.Fatalf("expected one build row, got %d", len(rows))
	}
	row := rows[0]
	if row.State != BldrDevtoolManifestStateRunning {
		t.Fatalf("expected running row, got %s", row.State)
	}
	if row.BuildTargets != "desktop" || row.TargetPlatformIDs != "browser,desktop/darwin/arm64" {
		t.Fatalf("expected finite build metadata, got %#v", row)
	}
	if !row.FullRebuild || row.HotRebuild || row.CacheHit {
		t.Fatalf("unexpected rebuild flags: %#v", row)
	}
	if row.WatchedFileCount != 3 {
		t.Fatalf("expected watched file count, got %d", row.WatchedFileCount)
	}
	if row.DependencyRebuildReason != "manifest dependency changed: core" {
		t.Fatalf("expected dependency rebuild reason, got %q", row.DependencyRebuildReason)
	}

	adapter.SetManifestBuilderStatus(bldr_project_controller.ManifestBuilderStatus{
		ID:                "build:web",
		BuildTargetIDs:    []string{"desktop"},
		ManifestID:        "web",
		PlatformID:        "browser",
		TargetPlatformIDs: []string{"browser"},
		BuildType:         "dev",
		RemoteID:          "devtool",
		State:             bldr_project_controller.ManifestBuilderStatusStateDone,
		CacheHit:          true,
		Summary:           "build complete",
	})

	rows = producer.GetStatus().GetManifestBuildRows()
	if len(rows) != 1 {
		t.Fatalf("expected row replacement, got %d rows", len(rows))
	}
	row = rows[0]
	if row.State != BldrDevtoolManifestStateReady || !row.CacheHit || row.Summary != "build complete" {
		t.Fatalf("expected successful cache-hit ready row, got %#v", row)
	}
	if row.BuildTargets != "desktop" || row.TargetPlatformIDs != "browser" {
		t.Fatalf("expected finite build completion metadata, got %#v", row)
	}

	adapter.SetManifestBuilderStatus(bldr_project_controller.ManifestBuilderStatus{
		ID:         "build:web",
		ManifestID: "web",
		PlatformID: "browser",
		BuildType:  "dev",
		RemoteID:   "devtool",
		State:      bldr_project_controller.ManifestBuilderStatusStateCanceled,
		Summary:    "build canceled",
		Error:      context.Canceled.Error(),
	})
	row = producer.GetStatus().GetManifestBuildRows()[0]
	if row.State != BldrDevtoolManifestStateCanceled {
		t.Fatalf("expected canceled row, got %#v", row)
	}
}

func TestManifestFetchRowsJoinRelatedLocalBuildRows(t *testing.T) {
	producer := NewBldrDevtoolStatusProducer(nil)

	producer.UpdateStatus(func(current *BldrDevtoolStatus) *BldrDevtoolStatus {
		return current.WithManifestFetchRows([]BldrDevtoolManifestFetchRow{{
			ID:         "fetch:web",
			ManifestID: "web",
			PlatformID: "browser,native",
			BuildType:  "dev",
			State:      BldrDevtoolManifestStateRunning,
		}})
	})
	producer.UpdateStatus(func(current *BldrDevtoolStatus) *BldrDevtoolStatus {
		return current.WithManifestBuildRows([]BldrDevtoolManifestBuildRow{{
			ID:         "build:web-browser",
			ManifestID: "web",
			PlatformID: "browser",
			BuildType:  "dev",
			RemoteID:   "devtool",
			State:      BldrDevtoolManifestStateRunning,
		}, {
			ID:         "build:web-native",
			ManifestID: "web",
			PlatformID: "native",
			BuildType:  "dev",
			RemoteID:   "devtool",
			State:      BldrDevtoolManifestStateQueued,
		}})
	})

	rows := producer.GetStatus().GetManifestFetchRows()
	if len(rows) != 1 {
		t.Fatalf("expected one fetch row, got %d", len(rows))
	}
	row := rows[0]
	if !row.BlockedOnLocalBuild {
		t.Fatalf("expected fetch row to be blocked on local build: %+v", row)
	}
	if row.LocalBuildIDs != "build:web-browser,build:web-native" {
		t.Fatalf("unexpected local build ids: %q", row.LocalBuildIDs)
	}
	if row.RemoteID != "devtool" {
		t.Fatalf("unexpected remote id: %q", row.RemoteID)
	}

	producer.UpdateStatus(func(current *BldrDevtoolStatus) *BldrDevtoolStatus {
		rows := current.GetManifestBuildRows()
		for idx := range rows {
			rows[idx].State = BldrDevtoolManifestStateReady
		}
		return current.WithManifestBuildRows(rows)
	})
	row = producer.GetStatus().GetManifestFetchRows()[0]
	if row.BlockedOnLocalBuild {
		t.Fatalf("expected ready local builds not to block fetch row: %+v", row)
	}
}

func TestPluginStatusAdapterPublishesSchedulerRows(t *testing.T) {
	producer := NewBldrDevtoolStatusProducer(nil)
	adapter := &pluginStatusAdapter{producer: producer}
	lastErrorAt := timestamp.New(time.Date(2026, 5, 8, 14, 15, 16, 17, time.UTC))

	adapter.setPluginStatusSnapshotRows(&plugin_host_scheduler.PluginStatusSnapshot{
		Plugins: []*bldr_plugin.PluginStatus{{
			PluginId:    "web",
			InstanceKey: "right",
			State:       bldr_plugin.PluginState_PluginState_RUNNING,
			Running:     true,
		}, {
			PluginId:         "notes",
			InstanceKey:      "left",
			State:            bldr_plugin.PluginState_PluginState_REQUESTED,
			LastErrorMessage: "download plugin manifest: copy failed",
			LastErrorAt:      lastErrorAt,
		}},
	})

	rows := producer.GetStatus().GetPluginRows()
	if len(rows) != 2 {
		t.Fatalf("expected two plugin rows, got %d", len(rows))
	}
	if rows[0].ID != "plugin:notes/left" || rows[0].State != BldrDevtoolPluginStateErrored {
		t.Fatalf("unexpected errored plugin row: %+v", rows[0])
	}
	if rows[0].Error != "download plugin manifest: copy failed" {
		t.Fatalf("unexpected plugin error: %q", rows[0].Error)
	}
	if rows[0].LastErrorAt != "2026-05-08T14:15:16.000000017Z" {
		t.Fatalf("unexpected last error timestamp: %q", rows[0].LastErrorAt)
	}
	if rows[1].ID != "plugin:web/right" || rows[1].State != BldrDevtoolPluginStateRunning {
		t.Fatalf("unexpected running plugin row: %+v", rows[1])
	}

	adapter.setPluginStatusSnapshotRows(&plugin_host_scheduler.PluginStatusSnapshot{})
	if rows := producer.GetStatus().GetPluginRows(); len(rows) != 0 {
		t.Fatalf("expected empty scheduler snapshot to clear plugin rows: %+v", rows)
	}
}

func TestAttachPluginStatusWatchesSchedulerContainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	producer := NewBldrDevtoolStatusProducer(nil)
	scheduler := plugin_host_scheduler.NewController(
		logrus.NewEntry(logrus.New()),
		nil,
		plugin_host_scheduler.NewConfig("engine", "devtool", "volume", "test", false, false, false),
	)
	statusCtr := scheduler.GetPluginStatusCtr()
	writableStatusCtr, ok := statusCtr.(interface {
		SetValue(*plugin_host_scheduler.PluginStatusSnapshot)
	})
	if !ok {
		t.Fatal("expected scheduler plugin status container to be writable in test")
	}

	AttachPluginStatus(ctx, producer, scheduler)
	writableStatusCtr.SetValue(&plugin_host_scheduler.PluginStatusSnapshot{
		Plugins: []*bldr_plugin.PluginStatus{{
			PluginId:    "notes",
			InstanceKey: "left",
			State:       bldr_plugin.PluginState_PluginState_RUNNING,
			Running:     true,
		}},
	})

	status := waitForStatus(t, ctx, producer, func(status *BldrDevtoolStatus) bool {
		rows := status.GetPluginRows()
		return len(rows) == 1 && rows[0].ID == "plugin:notes/left"
	})
	row := status.GetPluginRows()[0]
	if row.State != BldrDevtoolPluginStateRunning || row.Summary != "plugin running" {
		t.Fatalf("unexpected watched plugin row: %+v", row)
	}
}

func TestPluginStatusRowMapsSchedulerStates(t *testing.T) {
	tests := []struct {
		name  string
		input *bldr_plugin.PluginStatus
		want  BldrDevtoolPluginState
	}{{
		name:  "unknown",
		input: &bldr_plugin.PluginStatus{PluginId: "notes"},
		want:  BldrDevtoolPluginStateUnknown,
	}, {
		name:  "requested",
		input: &bldr_plugin.PluginStatus{PluginId: "notes", State: bldr_plugin.PluginState_PluginState_REQUESTED},
		want:  BldrDevtoolPluginStateRequested,
	}, {
		name:  "running",
		input: &bldr_plugin.PluginStatus{PluginId: "notes", State: bldr_plugin.PluginState_PluginState_RUNNING},
		want:  BldrDevtoolPluginStateRunning,
	}, {
		name:  "errored",
		input: &bldr_plugin.PluginStatus{PluginId: "notes", LastErrorMessage: "boom"},
		want:  BldrDevtoolPluginStateErrored,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pluginStatusRow(tt.input)
			if got.State != tt.want {
				t.Fatalf("expected %s, got %+v", tt.want, got)
			}
		})
	}
}

func TestBuildManifestFetchRowExtractsReadyRefs(t *testing.T) {
	meta := bldr_manifest.NewManifestMeta("web", bldr_manifest.BuildType_DEV, "browser", 1)
	ref := bldr_manifest.NewManifestRef(meta, &bucket.ObjectRef{BucketId: "ready-bucket"})
	row := buildManifestFetchRow(
		"fetch:web",
		bldr_manifest.NewFetchManifest("web", []bldr_manifest.BuildType{bldr_manifest.BuildType_DEV}, []string{"browser"}, 0),
		false,
		nil,
		[]directive.AttachedValue{
			directive.NewAttachedValue(1, bldr_manifest.NewFetchManifestValue([]*bldr_manifest.ManifestRef{ref})),
		},
	)

	if row.State != BldrDevtoolManifestStateReady {
		t.Fatalf("expected ready fetch row, got %s", row.State)
	}
	if row.ReadyRefCount != 1 || row.ReadyRefs == "" {
		t.Fatalf("expected ready refs on fetch row: %+v", row)
	}
	if row.Summary != "1 ready ref" {
		t.Fatalf("unexpected ready ref summary: %q", row.Summary)
	}
}

func TestBuildManifestFetchRowReportsResolverErrors(t *testing.T) {
	row := buildManifestFetchRow(
		"fetch:web",
		bldr_manifest.NewFetchManifest("web", nil, []string{"browser"}, 0),
		false,
		[]error{errors.New("resolver failed")},
		nil,
	)

	if row.State != BldrDevtoolManifestStateError {
		t.Fatalf("expected error fetch row, got %s", row.State)
	}
	if row.Error != "resolver failed" {
		t.Fatalf("unexpected resolver error: %q", row.Error)
	}
}

func TestBldrDevtoolStatusObserverInitialScan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b := newStatusObserverTestBus(t, ctx)
	producer := NewBldrDevtoolStatusProducer(nil)

	_, fetchRef, err := b.AddDirective(
		bldr_manifest.NewFetchManifest(
			"web",
			[]bldr_manifest.BuildType{bldr_manifest.BuildType_DEV},
			[]string{"browser"},
			0,
		),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer fetchRef.Release()

	_, controllerRef, err := b.AddDirective(
		resolver.NewLoadControllerWithConfig(&bldr_web_view_observer.Config{}),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer controllerRef.Release()

	observer := NewBldrDevtoolStatusObserver(b, producer)
	releaseObserver, err := b.AddController(ctx, observer, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseObserver()

	status := waitForStatus(t, ctx, producer, func(status *BldrDevtoolStatus) bool {
		return len(status.GetManifestFetchRows()) == 1 &&
			len(status.GetControllerRows()) == 1
	})

	if got := status.GetManifestFetchRows()[0]; got.ManifestID != "web" || got.PlatformID != "browser" {
		t.Fatalf("unexpected fetch row: %+v", got)
	}
	if got := status.GetPluginRows(); len(got) != 0 {
		t.Fatalf("expected scheduler-owned plugin rows to remain untouched, got %+v", got)
	}
	if got := status.GetControllerRows()[0]; got.ControllerID != bldr_web_view_observer.ConfigID || got.Kind != "load" {
		t.Fatalf("unexpected controller row: %+v", got)
	}
}

func TestBldrDevtoolStatusObserverDoesNotOverwriteSchedulerPluginRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b := newStatusObserverTestBus(t, ctx)
	producer := NewBldrDevtoolStatusProducer(nil)
	(&pluginStatusAdapter{producer: producer}).setPluginStatusSnapshotRows(&plugin_host_scheduler.PluginStatusSnapshot{
		Plugins: []*bldr_plugin.PluginStatus{{
			PluginId:    "notes",
			InstanceKey: "left",
			State:       bldr_plugin.PluginState_PluginState_REQUESTED,
		}},
	})

	observer := NewBldrDevtoolStatusObserver(b, producer)
	releaseObserver, err := b.AddController(ctx, observer, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseObserver()

	_, fetchRef, err := b.AddDirective(
		bldr_manifest.NewFetchManifest(
			"web",
			[]bldr_manifest.BuildType{bldr_manifest.BuildType_DEV},
			[]string{"browser"},
			0,
		),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer fetchRef.Release()

	status := waitForStatus(t, ctx, producer, func(status *BldrDevtoolStatus) bool {
		return len(status.GetManifestFetchRows()) == 1 &&
			len(status.GetPluginRows()) == 1
	})
	if got := status.GetPluginRows()[0]; got.PluginID != "notes" || got.InstanceKey != "left" {
		t.Fatalf("expected scheduler plugin row to remain intact, got %+v", got)
	}
}

func TestBldrDevtoolStatusObserverReleaseClosesObservedDirectives(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b := newStatusObserverTestBus(t, ctx)
	producer := NewBldrDevtoolStatusProducer(nil)

	di, fetchRef, err := b.AddDirective(
		bldr_manifest.NewFetchManifest(
			"web",
			[]bldr_manifest.BuildType{bldr_manifest.BuildType_DEV},
			[]string{"browser"},
			0,
		),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	observer := NewBldrDevtoolStatusObserver(b, producer)
	releaseObserver, err := b.AddController(ctx, observer, nil)
	if err != nil {
		t.Fatal(err)
	}

	waitForStatus(t, ctx, producer, func(status *BldrDevtoolStatus) bool {
		return len(status.GetManifestFetchRows()) == 1
	})

	releaseObserver()
	waitForStatus(t, ctx, producer, func(status *BldrDevtoolStatus) bool {
		return len(status.GetManifestFetchRows()) == 0
	})

	fetchRef.Release()
	if !di.CloseIfUnreferenced(false) {
		t.Fatal("expected released observer not to keep directive referenced")
	}
}

func TestBldrDevtoolStatusObserverCloseReleasesStateCallbacks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b := newStatusObserverTestBus(t, ctx)
	producer := NewBldrDevtoolStatusProducer(nil)

	di, fetchRef, err := b.AddDirective(
		bldr_manifest.NewFetchManifest(
			"web",
			[]bldr_manifest.BuildType{bldr_manifest.BuildType_DEV},
			[]string{"browser"},
			0,
		),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	observer := NewBldrDevtoolStatusObserver(b, producer)
	releaseObserver, err := b.AddController(ctx, observer, nil)
	if err != nil {
		t.Fatal(err)
	}

	waitForStatus(t, ctx, producer, func(status *BldrDevtoolStatus) bool {
		rows := status.GetManifestFetchRows()
		return len(rows) == 1 && rows[0].State == BldrDevtoolManifestStateQueued
	})

	releaseObserver()
	waitForStatus(t, ctx, producer, func(status *BldrDevtoolStatus) bool {
		return len(status.GetManifestFetchRows()) == 0
	})

	stateChanged := make(chan struct{}, 1)
	releaseState := di.AddStateCallback(func(_ bool, _ []error, vals []directive.AttachedValue) {
		if len(vals) != 0 {
			select {
			case stateChanged <- struct{}{}:
			default:
			}
		}
	})
	defer releaseState()

	fetchCtrl := &testFetchManifestController{}
	releaseFetchCtrl, err := b.AddController(ctx, fetchCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFetchCtrl()

	select {
	case <-stateChanged:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if rows := producer.GetStatus().GetManifestFetchRows(); len(rows) != 0 {
		t.Fatalf("expected closed observer not to publish after directive state change: %+v", rows)
	}

	fetchRef.Release()
	if !di.CloseIfUnreferenced(false) {
		t.Fatal("expected closed observer not to keep directive referenced")
	}
}

func TestBldrDevtoolStatusObserverDisposeCallbackRemovesKeyedDirective(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b := newStatusObserverTestBus(t, ctx)
	producer := NewBldrDevtoolStatusProducer(nil)

	di, fetchRef, err := b.AddDirective(
		bldr_manifest.NewFetchManifest(
			"web",
			[]bldr_manifest.BuildType{bldr_manifest.BuildType_DEV},
			[]string{"browser"},
			0,
		),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	observer := NewBldrDevtoolStatusObserver(b, producer)
	releaseObserver, err := b.AddController(ctx, observer, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseObserver()

	waitForStatus(t, ctx, producer, func(status *BldrDevtoolStatus) bool {
		return len(status.GetManifestFetchRows()) == 1
	})

	fetchRef.Release()
	if !di.CloseIfUnreferenced(false) {
		t.Fatal("expected directive to close after releasing its strong ref")
	}

	waitForStatus(t, ctx, producer, func(status *BldrDevtoolStatus) bool {
		return len(status.GetManifestFetchRows()) == 0
	})
	if keys := observer.observed.GetKeys(); len(keys) != 0 {
		t.Fatalf("expected dispose callback to remove keyed observer, got %v", keys)
	}
}

type testFetchManifestController struct{}

func (c *testFetchManifestController) GetControllerInfo() *cb_controller.Info {
	return cb_controller.NewInfo("bldr/devtool/status/test-fetch-manifest", controller.MustParseVersion("0.0.1"), "test")
}

func (c *testFetchManifestController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (c *testFetchManifestController) HandleDirective(
	_ context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	switch di.GetDirective().(type) {
	case bldr_manifest.FetchManifest:
		return directive.R(
			directive.NewValueResolver([]*bldr_manifest.FetchManifestValue{
				bldr_manifest.NewFetchManifestValue(nil),
			}),
			nil,
		)
	default:
		return nil, nil
	}
}

func (c *testFetchManifestController) Close() error {
	return nil
}

// _ is a type assertion
var _ cb_controller.Controller = (*testFetchManifestController)(nil)

func newStatusObserverTestBus(t *testing.T, ctx context.Context) bus.Bus {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func waitForStatus(
	t *testing.T,
	ctx context.Context,
	producer *BldrDevtoolStatusProducer,
	cond func(*BldrDevtoolStatus) bool,
) *BldrDevtoolStatus {
	t.Helper()
	for {
		current := producer.GetStatus()
		if cond(current) {
			return current
		}
		next, err := producer.GetStatusCtr().WaitValueChange(ctx, current, nil)
		if err != nil {
			t.Fatal(err)
		}
		if cond(next) {
			return next
		}
	}
}

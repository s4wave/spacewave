package status

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// BldrDevtoolStatusObserverControllerID is the controller id for the observer.
const BldrDevtoolStatusObserverControllerID = "bldr/devtool/status/observer"

// BldrDevtoolStatusObserverVersion is the observer controller version.
var BldrDevtoolStatusObserverVersion = semver.MustParse("0.0.1")

// BldrDevtoolStatusObserver observes ControllerBus directives and publishes status.
type BldrDevtoolStatusObserver struct {
	b        bus.Bus
	producer *BldrDevtoolStatusProducer

	mtx      sync.Mutex
	closed   bool
	observed map[string]*observedDirective

	manifestFetchRows map[string]BldrDevtoolManifestFetchRow
	controllerRows    map[string]BldrDevtoolControllerRow
}

// NewBldrDevtoolStatusObserver constructs a Bldr Devtool Status observer.
func NewBldrDevtoolStatusObserver(
	b bus.Bus,
	producer *BldrDevtoolStatusProducer,
) *BldrDevtoolStatusObserver {
	if producer == nil {
		producer = NewBldrDevtoolStatusProducer(nil)
	}
	return &BldrDevtoolStatusObserver{
		b:                 b,
		producer:          producer,
		observed:          make(map[string]*observedDirective),
		manifestFetchRows: make(map[string]BldrDevtoolManifestFetchRow),
		controllerRows:    make(map[string]BldrDevtoolControllerRow),
	}
}

// GetControllerInfo returns information about the controller.
func (o *BldrDevtoolStatusObserver) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		BldrDevtoolStatusObserverControllerID,
		BldrDevtoolStatusObserverVersion,
		"observes bldr devtool directive status",
	)
}

// GetStatusProducer returns the status producer.
func (o *BldrDevtoolStatusObserver) GetStatusProducer() *BldrDevtoolStatusProducer {
	return o.producer
}

// Execute observes initial directives and add/remove broadcasts.
func (o *BldrDevtoolStatusObserver) Execute(ctx context.Context) error {
	defer o.Close()

	o.rescanDirectives()
	bcast := o.b.GetDirectivesBroadcast()
	for {
		var waitCh <-chan struct{}
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})
		select {
		case <-ctx.Done():
			return nil
		case <-waitCh:
			o.rescanDirectives()
		}
	}
}

// HandleDirective does not resolve directives.
func (o *BldrDevtoolStatusObserver) HandleDirective(
	context.Context,
	directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases observer callbacks.
func (o *BldrDevtoolStatusObserver) Close() error {
	o.mtx.Lock()
	if o.closed {
		o.mtx.Unlock()
		return nil
	}
	o.closed = true
	observed := o.observed
	o.observed = make(map[string]*observedDirective)
	o.manifestFetchRows = make(map[string]BldrDevtoolManifestFetchRow)
	o.controllerRows = make(map[string]BldrDevtoolControllerRow)
	o.mtx.Unlock()

	for _, obs := range observed {
		obs.release()
	}
	o.publishSnapshot()
	return nil
}

func (o *BldrDevtoolStatusObserver) rescanDirectives() {
	o.mtx.Lock()
	if o.closed {
		o.mtx.Unlock()
		return
	}
	o.mtx.Unlock()

	activeKeys := make(map[string]struct{})
	for _, di := range o.b.GetDirectives() {
		obs, ok := o.buildObservedDirective(di)
		if !ok {
			continue
		}
		activeKeys[obs.key] = struct{}{}

		o.mtx.Lock()
		if o.closed {
			o.mtx.Unlock()
			return
		}
		_, exists := o.observed[obs.key]
		if exists {
			o.mtx.Unlock()
			continue
		}
		o.mtx.Unlock()

		obs.attach(di)

		o.mtx.Lock()
		if o.closed {
			o.mtx.Unlock()
			obs.release()
			return
		}
		if _, exists := o.observed[obs.key]; exists {
			o.mtx.Unlock()
			obs.release()
			continue
		}
		o.observed[obs.key] = obs
		o.mtx.Unlock()
	}

	var release []*observedDirective
	o.mtx.Lock()
	for key, obs := range o.observed {
		if _, ok := activeKeys[key]; ok {
			continue
		}
		delete(o.observed, key)
		o.deleteRowLocked(obs)
		release = append(release, obs)
	}
	o.mtx.Unlock()

	for _, obs := range release {
		obs.release()
	}
	o.publishSnapshot()
}

func (o *BldrDevtoolStatusObserver) buildObservedDirective(
	di directive.Instance,
) (*observedDirective, bool) {
	switch dir := di.GetDirective().(type) {
	case bldr_manifest.FetchManifest:
		key := "fetch:" + di.GetDirectiveIdent()
		return &observedDirective{
			key:  key,
			kind: observedDirectiveKindManifestFetch,
			update: func(isIdle bool, errs []error, vals []directive.AttachedValue) {
				o.setManifestFetchRow(buildManifestFetchRow(key, dir, isIdle, errs, vals))
			},
			dispose: func() {
				o.disposeObservedDirective(key)
			},
		}, true
	case resolver.LoadControllerWithConfig:
		key := "controller:load:" + di.GetDirectiveIdent()
		return &observedDirective{
			key:  key,
			kind: observedDirectiveKindController,
			update: func(isIdle bool, errs []error, vals []directive.AttachedValue) {
				o.setControllerRow(buildLoadControllerRow(key, dir, isIdle, errs, vals))
			},
			dispose: func() {
				o.disposeObservedDirective(key)
			},
		}, true
	case loader.ExecController:
		key := "controller:exec:" + di.GetDirectiveIdent()
		return &observedDirective{
			key:  key,
			kind: observedDirectiveKindController,
			update: func(isIdle bool, errs []error, vals []directive.AttachedValue) {
				o.setControllerRow(buildExecControllerRow(key, dir, isIdle, errs, vals))
			},
			dispose: func() {
				o.disposeObservedDirective(key)
			},
		}, true
	default:
		return nil, false
	}
}

func (o *BldrDevtoolStatusObserver) setManifestFetchRow(row BldrDevtoolManifestFetchRow) {
	o.mtx.Lock()
	if o.closed {
		o.mtx.Unlock()
		return
	}
	o.manifestFetchRows[row.ID] = row
	o.mtx.Unlock()
	o.publishSnapshot()
}

func (o *BldrDevtoolStatusObserver) setControllerRow(row BldrDevtoolControllerRow) {
	o.mtx.Lock()
	if o.closed {
		o.mtx.Unlock()
		return
	}
	o.controllerRows[row.ID] = row
	o.mtx.Unlock()
	o.publishSnapshot()
}

func (o *BldrDevtoolStatusObserver) disposeObservedDirective(key string) {
	var obs *observedDirective
	o.mtx.Lock()
	if o.closed {
		o.mtx.Unlock()
		return
	}
	if found := o.observed[key]; found != nil {
		obs = found
		delete(o.observed, key)
		o.deleteRowLocked(found)
	}
	o.mtx.Unlock()
	if obs != nil {
		obs.release()
	}
	o.publishSnapshot()
}

func (o *BldrDevtoolStatusObserver) deleteRowLocked(obs *observedDirective) {
	switch obs.kind {
	case observedDirectiveKindManifestFetch:
		delete(o.manifestFetchRows, obs.key)
	case observedDirectiveKindController:
		delete(o.controllerRows, obs.key)
	}
}

func (o *BldrDevtoolStatusObserver) publishSnapshot() {
	o.mtx.Lock()
	fetchRows := manifestFetchRowValues(o.manifestFetchRows)
	controllerRows := controllerRowValues(o.controllerRows)
	o.mtx.Unlock()

	slices.SortFunc(fetchRows, func(a, b BldrDevtoolManifestFetchRow) int {
		return strings.Compare(a.ID, b.ID)
	})
	slices.SortFunc(controllerRows, func(a, b BldrDevtoolControllerRow) int {
		return strings.Compare(a.ID, b.ID)
	})

	o.producer.UpdateStatus(func(current *BldrDevtoolStatus) *BldrDevtoolStatus {
		return current.
			WithManifestFetchRows(fetchRows).
			WithControllerRows(controllerRows)
	})
}

func buildManifestFetchRow(
	key string,
	dir bldr_manifest.FetchManifest,
	isIdle bool,
	errs []error,
	vals []directive.AttachedValue,
) BldrDevtoolManifestFetchRow {
	errText := errorSummary(errs)
	readyRefs := manifestFetchReadyRefs(vals)
	state := BldrDevtoolManifestStateRunning
	if errText != "" {
		state = BldrDevtoolManifestStateError
	} else if len(readyRefs) != 0 {
		state = BldrDevtoolManifestStateReady
	} else if isIdle {
		state = BldrDevtoolManifestStateQueued
	}
	return BldrDevtoolManifestFetchRow{
		ID:            key,
		ManifestID:    dir.GetManifestId(),
		PlatformID:    strings.Join(dir.GetPlatformIds(), ","),
		BuildType:     buildTypesString(dir.GetBuildTypes()),
		State:         state,
		ReadyRefCount: len(readyRefs),
		ReadyRefs:     strings.Join(readyRefs, ","),
		Summary:       valueCountSummary(len(readyRefs), "ready ref"),
		Error:         errText,
	}
}

func buildLoadControllerRow(
	key string,
	dir resolver.LoadControllerWithConfig,
	isIdle bool,
	errs []error,
	vals []directive.AttachedValue,
) BldrDevtoolControllerRow {
	return buildControllerRow(
		key,
		controllerConfigID(dir.GetLoadControllerConfig()),
		"load",
		isIdle,
		errs,
		vals,
	)
}

func buildExecControllerRow(
	key string,
	dir loader.ExecController,
	isIdle bool,
	errs []error,
	vals []directive.AttachedValue,
) BldrDevtoolControllerRow {
	return buildControllerRow(
		key,
		controllerConfigID(dir.GetExecControllerConfig()),
		"exec",
		isIdle,
		errs,
		vals,
	)
}

func buildControllerRow(
	key string,
	controllerID string,
	kind string,
	isIdle bool,
	errs []error,
	vals []directive.AttachedValue,
) BldrDevtoolControllerRow {
	errText := errorSummary(errs)
	running := false
	for _, val := range vals {
		execVal, ok := val.GetValue().(loader.ExecControllerValue)
		if !ok {
			continue
		}
		if execErr := execVal.GetError(); execErr != nil && errText == "" {
			errText = execErr.Error()
		}
		if execVal.GetController() != nil {
			running = true
		}
	}

	state := BldrDevtoolControllerStateRequested
	if errText != "" {
		state = BldrDevtoolControllerStateError
	} else if running {
		state = BldrDevtoolControllerStateRunning
	} else if isIdle {
		state = BldrDevtoolControllerStateIdle
	}

	return BldrDevtoolControllerRow{
		ID:           key,
		ControllerID: controllerID,
		Kind:         kind,
		State:        state,
		Summary:      valueCountSummary(len(vals), "controller value"),
		Error:        errText,
	}
}

func controllerConfigID(conf config.Config) string {
	if conf == nil {
		return ""
	}
	return conf.GetConfigID()
}

func buildTypesString(buildTypes []bldr_manifest.BuildType) string {
	if len(buildTypes) == 0 {
		return ""
	}
	parts := make([]string, len(buildTypes))
	for i, buildType := range buildTypes {
		parts[i] = buildType.String()
	}
	return strings.Join(parts, ",")
}

func errorSummary(errs []error) string {
	var parts []string
	for _, err := range errs {
		if err != nil {
			parts = append(parts, err.Error())
		}
	}
	return strings.Join(parts, "; ")
}

func valueCountSummary(count int, noun string) string {
	switch count {
	case 0:
		return ""
	case 1:
		return "1 " + noun
	default:
		return "multiple " + noun + "s"
	}
}

func manifestFetchReadyRefs(vals []directive.AttachedValue) []string {
	var refs []string
	for _, val := range vals {
		fetchVal, ok := val.GetValue().(*bldr_manifest.FetchManifestValue)
		if !ok {
			continue
		}
		for _, manifestRef := range fetchVal.GetManifestRefs() {
			ref := manifestRef.GetManifestRef()
			if ref == nil {
				continue
			}
			refString := ref.MarshalString()
			if refString != "" {
				refs = append(refs, refString)
			}
		}
	}
	return refs
}

func manifestFetchRowValues(rows map[string]BldrDevtoolManifestFetchRow) []BldrDevtoolManifestFetchRow {
	vals := make([]BldrDevtoolManifestFetchRow, 0, len(rows))
	for _, row := range rows {
		vals = append(vals, row)
	}
	return vals
}

func controllerRowValues(rows map[string]BldrDevtoolControllerRow) []BldrDevtoolControllerRow {
	vals := make([]BldrDevtoolControllerRow, 0, len(rows))
	for _, row := range rows {
		vals = append(vals, row)
	}
	return vals
}

// _ is a type assertion
var _ controller.Controller = ((*BldrDevtoolStatusObserver)(nil))

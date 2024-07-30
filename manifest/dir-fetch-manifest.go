package bldr_manifest

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/util/keyed"
)

// FetchManifest is a directive to fetch a manifest to storage.
//
// Value type: *FetchManifestValue
type FetchManifest interface {
	// Directive indicates FetchManifest is a directive.
	directive.Directive

	// FetchManifestMeta returns the manifest metadata to fetch.
	// Cannot be empty.
	FetchManifestMeta() *ManifestMeta
}

// fetchManifest implements FetchManifest
type fetchManifest struct {
	manifestMeta *ManifestMeta
}

// NewFetchManifest constructs a new FetchManifest directive.
func NewFetchManifest(manifestMeta *ManifestMeta) FetchManifest {
	return &fetchManifest{manifestMeta: manifestMeta}
}

// NewFetchManifestValue constructs a new FetchManifest result value.
func NewFetchManifestValue(manifestRef *ManifestRef) *FetchManifestValue {
	return &FetchManifestValue{
		ManifestRef: manifestRef,
	}
}

// ExFetchManifest executes the FetchManifest directive waiting for a single result.
//
// Selects the most recent result from the available set (highest revision).
func ExFetchManifest(
	ctx context.Context,
	b bus.Bus,
	manifestMeta *ManifestMeta,
	returnIfIdle bool,
) (*FetchManifestValue, error) {
	vals, _, ref, err := bus.ExecCollectValues[*FetchManifestValue](
		ctx,
		b,
		NewFetchManifest(manifestMeta),
		!returnIfIdle,
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer ref.Release()

	var selected *FetchManifestValue
	for _, val := range vals {
		if selected == nil || selected.GetManifestRef().GetMeta().GetRev() < val.GetManifestRef().GetMeta().GetRev() {
			selected = val
		}
	}

	return selected, nil
}

// NewTransformFetchManifestValueToSnapshot transforms *FetchManifestValue to *ManifestSnapshot.
//
// followRef should construct a bucket lookup cursor located at the FetchManifestValue.
func NewTransformFetchManifestValueToSnapshot(
	followRef func(ctx context.Context, val *FetchManifestValue) (*bucket_lookup.Cursor, error),
) func(
	ctx context.Context,
	val directive.TypedAttachedValue[*FetchManifestValue],
) (*ManifestSnapshot, bool, error) {
	return func(ctx context.Context, val directive.TypedAttachedValue[*FetchManifestValue]) (*ManifestSnapshot, bool, error) {
		manifestVal := val.GetValue()
		manifestRef := manifestVal.GetManifestRef()
		if manifestRef.GetEmpty() {
			return nil, false, nil
		}

		manifestBls, err := followRef(ctx, manifestVal)
		if err != nil {
			return nil, false, err
		}
		if manifestBls == nil {
			return nil, false, nil
		}
		defer manifestBls.Release()

		_, bcs := manifestBls.BuildTransaction(nil)
		manifest, err := UnmarshalManifest(ctx, bcs)
		if err != nil {
			return nil, false, err
		}
		if manifest == nil {
			return nil, false, nil
		}

		manifestObjRef := manifestRef.GetManifestRef()
		return &ManifestSnapshot{
			ManifestRef: manifestObjRef,
			Manifest:    manifest,
		}, true, nil
	}
}

// SelectLatestTransformedManifestSnapshot selects the latest revision from the set of vals.
func SelectLatestTransformedManifestSnapshot(vals []directive.TransformedAttachedValue[*FetchManifestValue, *ManifestSnapshot]) int {
	idx := -1
	var idxRev uint64
	for i, val := range vals {
		snapshot := val.GetTransformedValue()
		rev := snapshot.GetManifest().GetMeta().GetRev()
		if idx == -1 || rev > idxRev {
			idx = i
			idxRev = rev
		}
	}
	return idx
}

// FetchLatestManifestEffect watches a FetchManifest directive, resolves the
// Manifest for each result, selects the value with the highest revision, and
// calls the callback with the selected manifest version when it changes.
//
// followRef should construct a bucket lookup cursor located at the FetchManifestValue.
func FetchLatestManifestEffect(
	ctx context.Context,
	b bus.Bus,
	manifestMeta *ManifestMeta,
	followRef func(ctx context.Context, val *FetchManifestValue) (*bucket_lookup.Cursor, error),
	effect func(val directive.TransformedAttachedValue[*FetchManifestValue, *ManifestSnapshot]) func(),
	keyedOpts ...keyed.Option[uint32, directive.TypedAttachedValue[*FetchManifestValue]],
) (directive.Instance, directive.Reference, error) {
	return bus.ExecOneOffWatchTransformEffect[*FetchManifestValue, *ManifestSnapshot](
		ctx,
		NewTransformFetchManifestValueToSnapshot(followRef),
		SelectLatestTransformedManifestSnapshot,
		effect,
		b,
		NewFetchManifest(manifestMeta),
		keyedOpts...,
	)
}

// SelectLatestFetchManifestValue selects the FetchManifestValue with the highest rev.
//
// If there are no manifests, returns -1.
func SelectLatestFetchManifestValue(vals []directive.TypedAttachedValue[*FetchManifestValue]) int {
	var latestRev uint64
	var latestIdx int = -1
	for i, aval := range vals {
		val := aval.GetValue()
		rev := val.GetManifestRef().GetMeta().GetRev()
		if latestIdx == -1 || rev > latestRev {
			latestIdx, latestRev = i, rev
		}
	}
	return latestIdx
}

// WatchLatestManifestValue executes FetchManifest and calls the callback with the FetchManifestValue with the highest rev.
// If there is no value, calls callback with latest=nil.
func WatchLatestManifestValue(
	b bus.Bus,
	manifestMeta *ManifestMeta,
	cb func(latest directive.TypedAttachedValue[*FetchManifestValue]),
) (directive.Instance, directive.Reference, error) {
	return bus.ExecOneOffWatchSelectCb[*FetchManifestValue](
		b,
		NewFetchManifest(manifestMeta),
		SelectLatestFetchManifestValue,
		cb,
	)
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *fetchManifest) Validate() error {
	if d.manifestMeta.GetManifestId() == "" {
		return ErrEmptyManifestID
	}

	return nil
}

// GetValueFetchManifestOptions returns options relating to value handling.
func (d *fetchManifest) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Second * 1,
		UnrefDisposeEmptyImmediate: true,
	}
}

// FetchManifestMeta returns the manifest metadata.
func (d *fetchManifest) FetchManifestMeta() *ManifestMeta {
	return d.manifestMeta
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *fetchManifest) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(FetchManifest)
	if !ok {
		return false
	}

	a, b := d.FetchManifestMeta(), od.FetchManifestMeta()
	return a.GetBuildType() == b.GetBuildType() &&
		a.GetManifestId() == b.GetManifestId() &&
		a.GetPlatformId() == b.GetPlatformId()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *fetchManifest) Superceeds(other directive.Directive) bool {
	od, ok := other.(FetchManifest)
	if !ok {
		return false
	}

	return d.FetchManifestMeta().GetRev() > od.FetchManifestMeta().GetRev()
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *fetchManifest) GetName() string {
	return "FetchManifest"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *fetchManifest) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["manifest-id"] = []string{d.FetchManifestMeta().GetManifestId()}
	return vals
}

// _ is a type assertion
var _ FetchManifest = ((*fetchManifest)(nil))

package bldr_manifest

import (
	"context"
	"errors"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// FetchManifest is a directive to fetch a manifest to storage.
type FetchManifest interface {
	// Directive indicates FetchManifest is a directive.
	directive.Directive

	// FetchManifestMeta returns the manifest metadata to fetch.
	// Cannot be empty.
	FetchManifestMeta() *ManifestMeta
}

// FetchManifestValue is the result type for FetchManifest.
// Multiple results may be pushed to the directive.
type FetchManifestValue = *FetchManifestResponse

// fetchManifest implements FetchManifest
type fetchManifest struct {
	manifestMeta *ManifestMeta
}

// NewFetchManifest constructs a new FetchManifest directive.
func NewFetchManifest(manifestMeta *ManifestMeta) FetchManifest {
	return &fetchManifest{manifestMeta: manifestMeta}
}

// ExFetchManifest executes the FetchManifest directive.
func ExFetchManifest(
	ctx context.Context,
	b bus.Bus,
	manifestMeta *ManifestMeta,
	returnIfIdle bool,
) (FetchManifestValue, error) {
	av, _, avRef, err := bus.ExecOneOff(ctx, b, NewFetchManifest(manifestMeta), bus.ReturnIfIdle(returnIfIdle), nil)
	if err != nil {
		return nil, err
	}
	if avRef == nil {
		return nil, errors.New("fetch manifest returned empty result")
	}
	avRef.Release()
	val, ok := av.GetValue().(FetchManifestValue)
	if !ok {
		return nil, errors.New("fetch manifest directive returned invalid result type")
	}
	return val, nil
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
		UnrefDisposeDur: time.Second * 3,
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

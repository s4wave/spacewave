package unixfs_access

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// AccessUnixFS is a directive to access a unix filesystem.
// Multiple results may be pushed to the directive.
type AccessUnixFS interface {
	// Directive indicates AccessUnixFS is a directive.
	directive.Directive

	// AccessUnixFSID returns the filesystem ID to load.
	// Cannot be empty.
	AccessUnixFSID() string
}

// AccessUnixFSValue is the result type for AccessUnixFS.
// Returns a release function.
type AccessUnixFSValue = func(ctx context.Context) (*unixfs.FSHandle, func(), error)

// accessUnixFS implements AccessUnixFS
type accessUnixFS struct {
	unixFsID string
}

// NewAccessUnixFS constructs a new AccessUnixFS directive.
func NewAccessUnixFS(unixFsID string) AccessUnixFS {
	return &accessUnixFS{unixFsID: unixFsID}
}

// ExAccessUnixFS executes the AccessUnixFS directive.
// if returnIfIdle is set, returns nil, nil if not found.
func ExAccessUnixFS(
	ctx context.Context,
	b bus.Bus,
	unixFsID string,
	returnIfIdle bool,
	valDisposeCb func(),
) (AccessUnixFSValue, directive.Reference, error) {
	av, _, avRef, err := bus.ExecOneOff(ctx, b, NewAccessUnixFS(unixFsID), returnIfIdle, valDisposeCb)
	if err != nil {
		return nil, nil, err
	}
	if avRef == nil {
		return nil, nil, nil
	}
	val, valOk := av.GetValue().(AccessUnixFSValue)
	if !valOk {
		avRef.Release()
		return nil, nil, errors.New("access unixfs value invalid result")
	}
	return val, avRef, nil
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *accessUnixFS) Validate() error {
	if d.unixFsID == "" {
		return unixfs_errors.ErrEmptyUnixFsId
	}

	return nil
}

// GetValueAccessUnixFSOptions returns options relating to value handling.
func (d *accessUnixFS) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// AccessUnixFSID returns the plugin ID.
func (d *accessUnixFS) AccessUnixFSID() string {
	return d.unixFsID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *accessUnixFS) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(AccessUnixFS)
	if !ok {
		return false
	}

	if d.AccessUnixFSID() != od.AccessUnixFSID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *accessUnixFS) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *accessUnixFS) GetName() string {
	return "AccessUnixFS"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *accessUnixFS) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["unixfs-id"] = []string{d.AccessUnixFSID()}
	return vals
}

// _ is a type assertion
var _ AccessUnixFS = ((*accessUnixFS)(nil))

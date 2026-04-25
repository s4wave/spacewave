package session

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// MountSession is a directive to mount a provider account and session.
type MountSession interface {
	// Directive indicates MountSession is a directive.
	directive.Directive

	// MountSessionRef returns the session ref to mount.
	MountSessionRef() *SessionRef
}

// MountSessionValue is the result type for MountSession.
type MountSessionValue = Session

// ExMountSession executes a lookup for a single provider on the bus.
//
// If returnIfIdle is set, returns when the directive becomes idle.
func ExMountSession(
	ctx context.Context,
	b bus.Bus,
	ref *SessionRef,
	returnIfIdle bool,
	valDisposeCb func(),
) (Session, directive.Reference, error) {
	av, _, avRef, err := bus.ExecOneOffTyped[MountSessionValue](
		ctx,
		b,
		NewMountSession(ref),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCb,
	)
	if err != nil {
		return nil, nil, err
	}
	if av == nil {
		avRef.Release()
		return nil, nil, nil
	}
	return av.GetValue(), avRef, nil
}

// mountSession implements MountSession
type mountSession struct {
	sessionRef *SessionRef
}

// NewMountSession constructs a new MountSession directive.
func NewMountSession(ref *SessionRef) MountSession {
	return &mountSession{
		sessionRef: ref,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *mountSession) Validate() error {
	if err := d.sessionRef.Validate(); err != nil {
		return err
	}
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *mountSession) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// MountSessionRef returns the session to mount.
func (d *mountSession) MountSessionRef() *SessionRef {
	return d.sessionRef
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *mountSession) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(MountSession)
	if !ok {
		return false
	}

	return d.sessionRef.EqualVT(od.MountSessionRef())
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *mountSession) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *mountSession) GetName() string {
	return "MountSession"
}

// GetDebugVals returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *mountSession) GetDebugVals() directive.DebugValues {
	providerResourceRef := d.sessionRef.GetProviderResourceRef()
	return directive.DebugValues{
		"provider-id": []string{providerResourceRef.GetProviderId()},
		"account-id":  []string{providerResourceRef.GetProviderAccountId()},
		"session-id":  []string{providerResourceRef.GetId()},
	}
}

// _ is a type assertion
var _ MountSession = ((*mountSession)(nil))

package dex_solicit

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
)

// testRequestAccess exposes authority changes without a second role model.
type testRequestAccess struct {
	// ctx is the request context the access was authorized under. Like the
	// production access, Check fails once it ends.
	ctx context.Context
	// allowed holds the primitive authority decision supplied by the test.
	allowed *ccontainer.CContainer[bool]
	// watching reports that the request's revocation worker started.
	watching chan struct{}
	// released reports that the handler and revocation worker released authority.
	released chan struct{}
}

// newTestRequestAccess starts with readable authority for one request.
func newTestRequestAccess() *testRequestAccess {
	return &testRequestAccess{
		allowed:  ccontainer.NewCContainer[bool](true),
		watching: make(chan struct{}, 1), released: make(chan struct{}, 1),
	}
}

// Check fails once the request ends, then reads the current authority decision.
func (a *testRequestAccess) Check() error {
	if a.ctx != nil {
		if err := a.ctx.Err(); err != nil {
			return err
		}
	}
	if !a.allowed.GetValue() {
		return errors.New("test request access denied")
	}
	return nil
}

// WaitRevoked reports its start, then waits for authority to change or ctx to end.
func (a *testRequestAccess) WaitRevoked(ctx context.Context) error {
	// Let the test revoke only after the handler starts its watch.
	a.watching <- struct{}{}

	// Wait on the same container Check reads.
	_, err := a.allowed.WaitValueChange(ctx, true, nil)
	if err != nil {
		return err
	}
	return a.Check()
}

// Release reports that the handler no longer retains authority.
func (a *testRequestAccess) Release() {
	a.released <- struct{}{}
}

// _ is a type assertion.
var _ RequestAccess = (*testRequestAccess)(nil)

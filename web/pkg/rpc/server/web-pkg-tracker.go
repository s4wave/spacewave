package web_pkg_rpc_server

import (
	"context"

	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
)

// webPkgTracker tracks a LookupWebPkg directive and service.
type webPkgTracker struct {
	// c is the controller
	c *Controller
	// webPkgID is the web pkg identifier.
	webPkgID string
	// srvPromise contains the promise for the web pkg server.
	srvPromise *promise.PromiseContainer[*WebPkgServer]
}

// newWebPkgTracker constructs a new tracker routine.
func (c *Controller) newWebPkgTracker(key string) (keyed.Routine, *webPkgTracker) {
	tr := &webPkgTracker{
		c:          c,
		webPkgID:   key,
		srvPromise: promise.NewPromiseContainer[*WebPkgServer](),
	}
	return tr.execute, tr
}

// execute executes the tracker.
func (t *webPkgTracker) execute(ctx context.Context) error {
	webPkgID := t.webPkgID
	le := t.c.le.WithField("web-pkg-id", webPkgID)

	le.Debug("starting web pkg tracker")

	// we need to resolve the web pkg to construct the server.
	valCh, di, valRef, err := bus.ExecOneOffWatchCh(t.c.bus, web_pkg.NewLookupWebPkg(webPkgID))
	if err != nil {
		t.srvPromise.SetResult(nil, err)
		return err
	}
	defer valRef.Release()

	// if the directive becomes idle we will set the srvPromise value to nil.
	errCh := make(chan error, 1)
	di.AddIdleCallback(func(errs []error) {
		for _, err := range errs {
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
		}

		errCh <- nil
	})

	var val web_pkg.WebPkg
WaitLoop:
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-errCh:
			if err != nil {
				return err
			}
			if val == nil {
				t.srvPromise.SetResult(nil, nil)
			}
			continue
		case av := <-valCh:
			lvv, ok := av.GetValue().(web_pkg.WebPkg)
			if !ok || val == lvv {
				continue WaitLoop
			}
			val = lvv
			if val == nil {
				t.srvPromise.SetPromise(nil)
				continue
			}
		}

		srv := NewWebPkgServer(le, val)
		le.Debug("proxy web pkg ready")
		t.srvPromise.SetResult(srv, nil)
	}
}

package web_pkg_rpc_server

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/keyed"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_rpc "github.com/s4wave/spacewave/bldr/web/pkg/rpc"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// ControllerID is the web pkg rpc server controller id.
const ControllerID = "bldr/web/pkg/rpc/server"

// Version is the controller version.
var Version = controller.MustParseVersion("0.0.1")

// defServiceIDPrefix is the default service id prefix.
const defServiceIDPrefix = web_pkg_rpc.SRPCAccessWebPkgServiceID + "/"

// Controller implements the web pkg rpc server.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// cc is controller config
	cc *Config
	// serviceIdPrefix is the prefix to watch for rpc requests.
	// If empty, defaults to web.pkg.rpc.AccessWebPkg.
	serviceIdPrefix string
	// matchWebPkgIdRe is the regexp to match web pkg ids
	// if nil, match any
	matchWebPkgIdRe *regexp.Regexp
	// webPkgs is the list of web pkg trackers.
	webPkgs *keyed.KeyedRefCount[string, *webPkgTracker]
	// routines owns tracker execution after Execute returns.
	routines routineGroup
	// releaseDelay controls retention after a resolver releases a tracker.
	releaseDelay time.Duration
	// lifecycleMtx guards shutdown and delayed releases.
	lifecycleMtx sync.Mutex
	// closed rejects new tracker work after shutdown begins.
	closed bool
	// closeDone closes after all tracker and delayed release work stops.
	closeDone chan struct{}
	// delayedReleases owns retained tracker references.
	delayedReleases map[*keyed.KeyedRef[string, *webPkgTracker]]*time.Timer
	// delayedWG joins delayed release callbacks.
	delayedWG sync.WaitGroup
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	cc *Config,
) (*Controller, error) {
	webPkgIdRe, err := cc.ParseWebPkgIdRe()
	if err != nil {
		return nil, err
	}

	releaseDelay, err := cc.ParseReleaseDelay()
	if err != nil {
		return nil, err
	}

	serviceIDPrefix := cc.GetServiceIdPrefix()
	if serviceIDPrefix == "" {
		serviceIDPrefix = defServiceIDPrefix
	} else if serviceIDPrefix[len(serviceIDPrefix)-1] != '/' {
		// must end with / if using a prefix
		serviceIDPrefix += "/"
	}

	c := &Controller{
		le:              le,
		cc:              cc,
		bus:             bus,
		serviceIdPrefix: serviceIDPrefix,
		matchWebPkgIdRe: webPkgIdRe,
		releaseDelay:    releaseDelay,
		delayedReleases: make(map[*keyed.KeyedRef[string, *webPkgTracker]]*time.Timer),
	}

	c.webPkgs = keyed.NewKeyedRefCount(
		func(key string) (keyed.Routine, *webPkgTracker) {
			r, tracker := c.newWebPkgTracker(key)
			return c.routines.wrap(r), tracker
		},
		keyed.WithExitLogger[string, *webPkgTracker](le),
		keyed.WithBackoff[string, *webPkgTracker](func(k string) cbackoff.BackOff {
			if cc.GetBackoff().SizeVT() == 0 {
				return (&backoff.Backoff{Exponential: &backoff.Exponential{
					InitialInterval: 200,
					MaxInterval:     1000,
				}}).Construct()
			}
			return cc.GetBackoff().Construct()
		}),
	)

	return c, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "web pkg server")
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	c.lifecycleMtx.Lock()
	defer c.lifecycleMtx.Unlock()
	if c.closed {
		return context.Canceled
	}
	c.webPkgs.SetContext(ctx, true)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	c.lifecycleMtx.Lock()
	closed := c.closed
	c.lifecycleMtx.Unlock()
	if closed {
		return nil, nil
	}
	dir := di.GetDirective()
	switch d := dir.(type) {
	case bifrost_rpc.LookupRpcService:
		serviceID := d.LookupRpcServiceID()

		// check the configured service id prefix (may be empty)
		webPkgID, strippedPrefix := srpc.CheckStripPrefix(serviceID, []string{c.serviceIdPrefix})
		if strippedPrefix == "" {
			// prefix mismatch
			break
		}

		// validate the web pkg id
		if err := web_pkg.ValidateWebPkgId(webPkgID); err != nil {
			c.le.
				WithField("web-pkg-id", webPkgID).
				Warn("ignoring invalid web pkg id in service name")
			break
		}

		// check the filters
		webPkgIDList := c.cc.GetWebPkgIdList()
		webPkgIDPrefixList := c.cc.GetWebPkgIdPrefixes()
		webPkgIDRe := c.matchWebPkgIdRe
		matched := len(webPkgIDList) == 0 && len(webPkgIDPrefixList) == 0 && webPkgIDRe == nil
		if !matched && len(webPkgIDPrefixList) != 0 {
			for _, prefix := range webPkgIDPrefixList {
				if strings.HasPrefix(webPkgID, prefix) {
					matched = true
					break
				}
			}
		}
		if !matched && webPkgIDRe != nil {
			matched = webPkgIDRe.MatchString(webPkgID)
		}
		if !matched {
			if slices.Contains(webPkgIDList, webPkgID) {
				matched = true
			}
		}
		if !matched {
			return nil, nil
		}

		// resolve with the refcount
		return directive.R(&webPkgResolver{
			c:   c,
			key: webPkgID,
			buildValue: func(ctx context.Context, val *webPkgTracker) (directive.Value, error) {
				if val == nil {
					return nil, nil
				}

				res, err := val.srvPromise.Await(ctx)
				if err != nil {
					return nil, err
				}
				if res == nil {
					return nil, nil
				}

				var rval bifrost_rpc.LookupRpcServiceValue = web_pkg_rpc.NewSRPCAccessWebPkgHandler(res, serviceID)
				return rval, nil
			},
		}, nil)
	}

	return nil, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	c.lifecycleMtx.Lock()
	if c.closed {
		done := c.closeDone
		c.lifecycleMtx.Unlock()
		<-done
		return nil
	}
	c.closed = true
	c.closeDone = make(chan struct{})
	done := c.closeDone
	c.routines.stopAccepting()
	c.lifecycleMtx.Unlock()

	c.webPkgs.ClearContext()
	c.stopDelayedReleases()
	c.routines.wait()
	c.delayedWG.Wait()
	close(done)
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))

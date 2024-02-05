package web_pkg_rpc_server

import (
	"context"
	"regexp"
	"strings"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	web_pkg_rpc "github.com/aperturerobotics/bldr/web/pkg/rpc"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver"
	cbackoff "github.com/cenkalti/backoff"
	"github.com/sirupsen/logrus"
)

// ControllerID is the web pkg rpc server controller id.
const ControllerID = "bldr/web/pkg/rpc/server"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

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
	}

	c.webPkgs = keyed.NewKeyedRefCount(
		c.newWebPkgTracker,
		keyed.WithExitLogger[string, *webPkgTracker](le),
		keyed.WithReleaseDelay[string, *webPkgTracker](releaseDelay),
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

// Execute executes the given controller.
func (c *Controller) Execute(ctx context.Context) error {
	c.webPkgs.SetContext(ctx, true)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
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
			for _, mWebPkgID := range webPkgIDList {
				if mWebPkgID == webPkgID {
					matched = true
					break
				}
			}
		}
		if !matched {
			return nil, nil
		}

		// resolve with the refcount
		return directive.R(directive.NewKeyedRefCountResolver(
			c.webPkgs,
			webPkgID,
			true,
			func(ctx context.Context, val *webPkgTracker) (directive.Value, error) {
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
		), nil)
	}

	return nil, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))

package web_pkg_rpc_client

import (
	"context"
	"regexp"
	"slices"
	"strings"

	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_rpc "github.com/s4wave/spacewave/bldr/web/pkg/rpc"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID is the web pkg rpc client controller id.
const ControllerID = "bldr/web/pkg/rpc/client"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// Controller implements the web pkg rpc client.
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

	serviceIDPrefix := cc.GetServiceIdPrefix()
	if serviceIDPrefix == "" {
		serviceIDPrefix = web_pkg_rpc.DefServiceIDPrefix
	} else if serviceIDPrefix[len(serviceIDPrefix)-1] != '/' {
		// must end with / if using a prefix
		serviceIDPrefix += "/"
	}

	return &Controller{
		le:              le,
		cc:              cc,
		bus:             bus,
		serviceIdPrefix: serviceIDPrefix,
		matchWebPkgIdRe: webPkgIdRe,
	}, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "web pkg server")
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case web_pkg.LookupWebPkg:
		webPkgID := d.LookupWebPkgID()

		// validate the web pkg id
		if err := web_pkg.ValidateWebPkgId(webPkgID); err != nil {
			c.le.
				WithField("web-pkg-id", webPkgID).
				Warn("ignoring invalid web pkg id")
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

		// resolve
		return directive.R(newResolveLookupWebPkg(c, webPkgID), nil)
	}

	return nil, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))

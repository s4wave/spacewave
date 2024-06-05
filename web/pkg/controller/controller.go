package web_pkg_controller

import (
	"context"

	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

// WebPkgGetter is a function to resolve a web package.
//
// release is a function to call if the web pkg is released.
// return nil, nil, nil for not found.
// return a release function if necessary
type WebPkgGetter = directive.KeyedGetterFunc[string, web_pkg.LookupWebPkgValue] // func(ctx context.Context, webPkgID string, release func()) (web_pkg.LookupWebPkgValue, func(), error)

// Controller implements the generic web pkg controller.
//
// Wraps a getter function and resolves LookupWebPkg.
// Can be used with the static web pkg implementation.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// info is the controller info
	info *controller.Info
	// getter is the web pkg getter
	getter WebPkgGetter
	// webPkgIds is the list of web package ids.
	// if empty, passes all web pkg ids to the getter.
	webPkgIds []string
}

// NewController constructs a new web pkg controller.
//
// webPkgIds is the list of web package ids.
// if empty, passes all web pkg ids to the getter.
func NewController(
	le *logrus.Entry,
	info *controller.Info,
	getter WebPkgGetter,
	webPkgIds []string,
) *Controller {
	return &Controller{
		le:        le,
		info:      info,
		getter:    getter,
		webPkgIds: webPkgIds,
	}
}

// NewControllerWithWebPkg constructs a new controller with a static WebPkg.
func NewControllerWithWebPkg(le *logrus.Entry, info *controller.Info, webPkg web_pkg.WebPkg) *Controller {
	id := webPkg.GetId()
	return NewController(
		le,
		info,
		func(ctx context.Context, key string, released func()) (web_pkg.LookupWebPkgValue, func(), error) {
			if key != id {
				return nil, nil, nil
			}
			return webPkg, nil, nil
		},
		[]string{id},
	)
}

// NewControllerWithWebPkgList constructs a new controller with a list of WebPkg.
func NewControllerWithWebPkgList(le *logrus.Entry, info *controller.Info, webPkgList []web_pkg.WebPkg) *Controller {
	webPkgs := make(map[string]web_pkg.WebPkg, len(webPkgList))
	for _, pkg := range webPkgList {
		if pkg == nil {
			continue
		}
		id := pkg.GetId()
		if id == "" {
			continue
		}
		webPkgs[id] = pkg
	}

	return NewControllerWithWebPkgMap(le, info, webPkgs)
}

// NewControllerWithWebPkgMap constructs a new controller with a map of WebPkg.
func NewControllerWithWebPkgMap(le *logrus.Entry, info *controller.Info, webPkgMap map[string]web_pkg.WebPkg) *Controller {
	webPkgsIds := maps.Keys(webPkgMap)
	slices.Sort(webPkgsIds)
	if len(webPkgsIds) != 0 && webPkgsIds[0] == "" {
		webPkgsIds = webPkgsIds[1:]
	}

	return NewController(
		le,
		info,
		func(ctx context.Context, key string, released func()) (web_pkg.LookupWebPkgValue, func(), error) {
			if key == "" {
				return nil, nil, nil
			}
			return webPkgMap[key], nil, nil
		},
		webPkgsIds,
	)
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
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
		return directive.R(c.resolveLookupWebPkg(ctx, di, d))
	}

	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.info
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))

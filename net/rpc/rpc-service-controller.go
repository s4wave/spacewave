package bifrost_rpc

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
)

// RpcServiceController resolves LookupRpcService with a srpc.Invoker.
type RpcServiceController struct {
	// info is the controller info
	info *controller.Info
	// handleCtr contains the rpc handler
	handleCtr *ccontainer.CContainer[srpc.Invoker]
	// errCtr contains any error building the handler
	errCtr *ccontainer.CContainer[*error]
	// rc is the refcount container
	rc *refcount.RefCount[srpc.Invoker]
	// serviceIdPrefixes is the list of service id prefixes to match.
	// ignores if empty
	serviceIdPrefixes []string
	// stripServiceIdPrefix removes the first matched serviceIdPrefix from the service id.
	// ignored if serviceIdPrefixes is empty
	stripServiceIdPrefix bool
	// serviceIdRe is a regex to match serviceIds.
	// ignores if empty
	serviceIdRe *regexp.Regexp
	// serviceIdList is a list of service ids to match.
	// ignores if empty
	serviceIdList []string
	// serverIdRe is a regex to match server ids.
	// ignores if empty
	serverIdRe *regexp.Regexp
}

// NewRpcServiceController constructs a new LookupRpcService resolver controller.
//
// Responds if a URL matches either serviceIdPrefixes OR serviceIdRe OR serviceIdList.
// all filters can be empty
// if no filters are set, resolves for any LookupRpcService directive.
// serverIdRe MUST match if set, regardless of the other filters.
func NewRpcServiceController(
	info *controller.Info,
	resolver RpcServiceBuilder,
	serviceIdPrefixes []string,
	stripServiceIdPrefix bool,
	serviceIdRe *regexp.Regexp,
	serviceIdList []string,
	serverIdRe *regexp.Regexp,
) *RpcServiceController {
	h := &RpcServiceController{
		info:                 info,
		handleCtr:            ccontainer.NewCContainer[srpc.Invoker](nil),
		errCtr:               ccontainer.NewCContainer[*error](nil),
		serviceIdPrefixes:    serviceIdPrefixes,
		stripServiceIdPrefix: stripServiceIdPrefix,
		serviceIdRe:          serviceIdRe,
		serviceIdList:        serviceIdList,
		serverIdRe:           serverIdRe,
	}
	h.rc = refcount.NewRefCount[srpc.Invoker](nil, false, h.handleCtr, h.errCtr, resolver)
	return h
}

// GetControllerInfo returns information about the controller.
func (c *RpcServiceController) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// Execute executes the controller.
func (c *RpcServiceController) Execute(ctx context.Context) error {
	c.rc.SetContext(ctx)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *RpcServiceController) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case LookupRpcService:
		serviceID := d.LookupRpcServiceID()
		// if we have no filters, match all.
		matched := len(c.serviceIdPrefixes) == 0 && c.serviceIdRe == nil && len(c.serviceIdList) == 0
		if !matched && len(c.serviceIdPrefixes) != 0 {
			for _, prefix := range c.serviceIdPrefixes {
				if strings.HasPrefix(serviceID, prefix) {
					matched = true
					break
				}
			}
		}
		if !matched && c.serviceIdRe != nil {
			matched = c.serviceIdRe.MatchString(serviceID)
		}
		if !matched {
			if slices.Contains(c.serviceIdList, serviceID) {
				matched = true
			}
		}
		if matched && c.serverIdRe != nil {
			serverID := d.LookupRpcServerID()
			matched = c.serverIdRe.MatchString(serverID)
		}
		if !matched {
			return nil, nil
		}
		return directive.R(
			directive.NewRefCountResolverWithXfrm(
				c.rc,
				func(ctx context.Context, val srpc.Invoker) (directive.Value, error) {
					if val == nil {
						return nil, nil
					}
					var invoker LookupRpcServiceValue = val //nolint:staticcheck
					if c.stripServiceIdPrefix {
						invoker = srpc.NewPrefixInvoker(invoker, c.serviceIdPrefixes)
					}
					return invoker, nil
				},
			),
			nil,
		)
	}
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *RpcServiceController) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*RpcServiceController)(nil))

package bifrost_rpc_access

import (
	"context"
	"regexp"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	backoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/refcount"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// ClientController resolves LookupRpcService with an AccessRpcService client.
type ClientController struct {
	le      *logrus.Entry
	info    *controller.Info
	svc     AccessClientFunc
	waitAck bool
	backoff backoff.BackOff

	clientRc *refcount.RefCount[SRPCAccessRpcServiceClient]

	serviceIDRe *regexp.Regexp
	serverIDRe  *regexp.Regexp
}

// AccessClientFunc is a function to access the AccessRpcServiceClient.
// The client should be released after the function returns.
// Released is a function to call when the value is no longer valid.
// Returns a release function.
// If the client is nil, an err must be returned.
type AccessClientFunc func(
	ctx context.Context,
	released func(),
) (SRPCAccessRpcServiceClient, func(), error)

// NewAccessClientFunc constructs a AccessClientFunc with a static client.
func NewAccessClientFunc(svc SRPCAccessRpcServiceClient) AccessClientFunc {
	return func(
		ctx context.Context,
		released func(),
	) (SRPCAccessRpcServiceClient, func(), error) {
		return svc, nil, nil
	}
}

// NewClientController constructs the controller.
// The regex fields can both be nil to accept any.
//
// if waitAck is set, waits for ack from the remote before starting the proxied rpc.
// note: usually you do not need waitAck set to true.
//
// if backoff is nil, uses a default backoff for retrying the rpc call.
func NewClientController(
	le *logrus.Entry,
	info *controller.Info,
	svc AccessClientFunc,
	serviceIDRe *regexp.Regexp,
	serverIDRe *regexp.Regexp,
	waitAck bool,
	bo backoff.BackOff,
) *ClientController {
	if bo == nil {
		exb := backoff.NewExponentialBackOff()
		exb.MaxElapsedTime = 0
		exb.MaxInterval = time.Second * 5
		bo = exb
	}
	c := &ClientController{
		le:          le,
		info:        info,
		svc:         svc,
		serviceIDRe: serviceIDRe,
		serverIDRe:  serverIDRe,
		waitAck:     waitAck,
		backoff:     bo,
	}
	c.clientRc = refcount.NewRefCount(
		nil, false, nil, nil,
		func(ctx context.Context, released func()) (SRPCAccessRpcServiceClient, func(), error) {
			val, rel, err := svc(ctx, released)
			if err != nil || val == nil {
				return nil, nil, err
			}
			return val, rel, nil
		},
	)
	return c
}

// GetControllerInfo returns the controller info.
func (c *ClientController) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// Execute executes the controller goroutine.
func (c *ClientController) Execute(ctx context.Context) error {
	c.clientRc.SetContext(ctx)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *ClientController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		// filter by regex
		if c.serviceIDRe != nil {
			serviceID := dir.LookupRpcServiceID()
			if serviceID != "" && !c.serviceIDRe.MatchString(serviceID) {
				return nil, nil
			}
		}
		if c.serverIDRe != nil {
			serverID := dir.LookupRpcServerID()
			if serverID != "" && !c.serverIDRe.MatchString(serverID) {
				return nil, nil
			}
		}

		// only returns an error if the RPC call failed.
		// if the service doesn't exist on the remote, it does not return an error.
		res := directive.NewRetryResolver(c.le, NewLookupRpcServiceResolver(
			dir,
			c.AccessClient,
			c.waitAck,
		), c.backoff)

		return directive.R(res, nil)
	}
	return nil, nil
}

// AccessClient adds a reference to the client and waits for it to be built.
// The released function will be called if the value was released.
func (c *ClientController) AccessClient(
	ctx context.Context,
	released func(),
) (SRPCAccessRpcServiceClient, func(), error) {
	return c.clientRc.ResolveWithReleased(ctx, released)
}

// Close releases any resources used by the controller.
func (c *ClientController) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller = ((*ClientController)(nil))
	_ AccessClientFunc      = ((*ClientController)(nil)).AccessClient
)

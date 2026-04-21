package bifrost_rpc

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
)

// LookupRpcService is a directive to lookup a RPC service for a server.
type LookupRpcService interface {
	// Directive indicates LookupRpcService is a directive.
	directive.Directive

	// LookupRpcServiceID returns the service ID to load.
	// Cannot be empty.
	LookupRpcServiceID() string
	// LookupRpcServerID returns the ID of the server requesting the service.
	// Use this for call routing only, not authentication.
	// Can be empty.
	LookupRpcServerID() string
}

// LookupRpcServiceValue is the result type for LookupRpcService.
// Multiple results may be pushed to the directive.
type LookupRpcServiceValue = srpc.Invoker

// LookupRpcServiceResolver resolves LookupRpcService with an Invoker.
type LookupRpcServiceResolver = *directive.ValueResolver[LookupRpcServiceValue]

// NewLookupRpcServiceResolver constructs a new LookupRpcServiceResolver directive.
func NewLookupRpcServiceResolver(invoker srpc.Invoker) LookupRpcServiceResolver {
	return directive.NewValueResolver([]LookupRpcServiceValue{invoker})
}

// lookupRpcService implements LookupRpcService
type lookupRpcService struct {
	serviceID string
	serverID  string
}

// NewLookupRpcService constructs a new LookupRpcService directive.
func NewLookupRpcService(serviceID, serverID string) LookupRpcService {
	return &lookupRpcService{serviceID: serviceID, serverID: serverID}
}

// ExLookupRpcService executes the LookupRpcService directive.
// Returns if the directive becomes idle (most likely: service not found).
// If no values are returned, returns nil, nil, nil
// If values are returned, returns vals, valsRef, nil
// Otherwise returns nil, nil, err
// If waitOne is set, waits for at least one value before returning.
// valDisposeCb is called if any of the values are no longer valid.
// valDisposeCb might be called multiple times.
func ExLookupRpcService(
	ctx context.Context,
	b bus.Bus,
	serviceID, serverID string,
	waitOne bool,
	valDisposeCb func(),
) ([]LookupRpcServiceValue, directive.Instance, directive.Reference, error) {
	out, di, valsRef, err := bus.ExecCollectValuesWithFilter(
		ctx,
		b,
		NewLookupRpcService(serviceID, serverID),
		waitOne,
		valDisposeCb,
		func(val LookupRpcServiceValue) (bool, error) {
			// If it's possible to query, do so.
			queryable, ok := val.(srpc.QueryableInvoker)
			if !ok {
				// keep it if it's not queryable
				return true, nil
			}
			return queryable.HasService(serviceID), nil
		},
	)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(out) == 0 {
		valsRef.Release()
		return nil, nil, nil, nil
	}
	return out, di, valsRef, nil
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupRpcService) Validate() error {
	if d.serviceID == "" {
		return srpc.ErrEmptyServiceID
	}

	return nil
}

// GetValueLookupRpcServiceOptions returns options relating to value handling.
func (d *lookupRpcService) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupRpcServiceID returns the plugin ID.
func (d *lookupRpcService) LookupRpcServiceID() string {
	return d.serviceID
}

// LookupRpcServerID returns the ID of the server requesting the service.
func (d *lookupRpcService) LookupRpcServerID() string {
	return d.serverID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupRpcService) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupRpcService)
	if !ok {
		return false
	}

	if d.LookupRpcServiceID() != od.LookupRpcServiceID() {
		return false
	}

	if d.LookupRpcServerID() != od.LookupRpcServerID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupRpcService) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupRpcService) GetName() string {
	return "LookupRpcService"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupRpcService) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["service-id"] = []string{d.LookupRpcServiceID()}
	if serverID := d.LookupRpcServerID(); serverID != "" {
		vals["server-id"] = []string{serverID}
	}
	return vals
}

// _ is a type assertion
var _ LookupRpcService = ((*lookupRpcService)(nil))

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

// lookupRpcService implements LookupRpcService.
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
// With no values, all four results are nil.
// With values, returns the values, instance, and a reference the caller releases.
// On failure, only the error result is non-nil.
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
			// Queryable invokers must advertise the requested service.
			queryable, ok := val.(srpc.QueryableInvoker)
			if !ok {
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

// GetValueOptions returns options relating to value handling.
func (d *lookupRpcService) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// Retain resolved and unresolved lookups between nearby RPC calls.
		// The bus cancels disposal when a new reference arrives.
		UnrefDisposeDur: time.Second,
	}
}

// LookupRpcServiceID returns the service ID to load.
func (d *lookupRpcService) LookupRpcServiceID() string {
	return d.serviceID
}

// LookupRpcServerID returns the ID of the server requesting the service.
func (d *lookupRpcService) LookupRpcServerID() string {
	return d.serverID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not supersede the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupRpcService) IsEquivalent(other directive.Directive) bool {
	// Require another service lookup before comparing its routing keys.
	od, ok := other.(LookupRpcService)
	if !ok {
		return false
	}

	// Require the same service before sharing the lookup.
	if d.LookupRpcServiceID() != od.LookupRpcServiceID() {
		return false
	}

	// Require the same server routing key before sharing the lookup.
	if d.LookupRpcServerID() != od.LookupRpcServerID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superseded.
func (d *lookupRpcService) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupRpcService) GetName() string {
	return "LookupRpcService"
}

// GetDebugVals returns the directive arguments stringified.
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

// _ verifies the directive contract.
var _ LookupRpcService = (*lookupRpcService)(nil)

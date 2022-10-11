package bldr_rpc

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
)

// LookupRpcService is a directive to lookup a RPC service for a client.
type LookupRpcService interface {
	// Directive indicates LookupRpcService is a directive.
	directive.Directive

	// LookupRpcServiceID returns the service ID to load.
	// Cannot be empty.
	LookupRpcServiceID() string
	// LookupRpcClientID returns the ID of the client requesting the service.
	// Use this for call routing only, not authentication.
	// Can be empty.
	LookupRpcClientID() string
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
	clientID  string
}

// NewLookupRpcService constructs a new LookupRpcService directive.
func NewLookupRpcService(serviceID, clientID string) LookupRpcService {
	return &lookupRpcService{serviceID: serviceID, clientID: clientID}
}

// ExLookupRpcService executes the LookupRpcService directive.
// Returns if the directive becomes idle (most likely: service not found).
// If no values are returned, returns nil, nil, nil
// If values are returned, returns vals, valsRef, nil
// Otherwise returns nil, nil, err
func ExLookupRpcService(
	ctx context.Context,
	b bus.Bus,
	serviceID, clientID string,
) ([]LookupRpcServiceValue, directive.Reference, error) {
	vals, valsRef, err := bus.ExecCollectValues(ctx, b, NewLookupRpcService(serviceID, clientID), nil)
	if err != nil {
		return nil, nil, err
	}
	out := make([]LookupRpcServiceValue, 0, len(vals))
	for _, val := range vals {
		v, ok := val.(LookupRpcServiceValue)
		if ok {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		valsRef.Release()
		return nil, nil, nil
	}
	return out, valsRef, nil
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
		UnrefDisposeDur: time.Second * 3,
	}
}

// LookupRpcServiceID returns the plugin ID.
func (d *lookupRpcService) LookupRpcServiceID() string {
	return d.serviceID
}

// LookupRpcClientID returns the ID of the client requesting the service.
func (d *lookupRpcService) LookupRpcClientID() string {
	return d.clientID
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

	if d.LookupRpcClientID() != od.LookupRpcClientID() {
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
	if clientID := d.LookupRpcClientID(); clientID != "" {
		vals["client-id"] = []string{clientID}
	}
	return vals
}

// _ is a type assertion
var _ LookupRpcService = ((*lookupRpcService)(nil))

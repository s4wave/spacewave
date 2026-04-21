package bifrost_rpc

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
)

// LookupRpcClient is a directive to lookup a RPC client for a service.
type LookupRpcClient interface {
	// Directive indicates LookupRpcClient is a directive.
	directive.Directive

	// LookupRpcServiceID returns the ID of the service.
	// Cannot be empty.
	LookupRpcServiceID() string
	// LookupRpcClientID returns the identifier of the caller.
	// Use this for call routing only, not authentication.
	// Can be empty.
	LookupRpcClientID() string
}

// LookupRpcClientValue is the result type for LookupRpcClient.
// Multiple results may be pushed to the directive.
type LookupRpcClientValue = srpc.Client

// LookupRpcClientResolver resolves LookupRpcClient with an Invoker.
type LookupRpcClientResolver = *directive.ValueResolver[LookupRpcClientValue]

// NewLookupRpcClientResolver constructs a new LookupRpcClientResolver directive.
func NewLookupRpcClientResolver(client srpc.Client) LookupRpcClientResolver {
	return directive.NewValueResolver([]LookupRpcClientValue{client})
}

// lookupRpcClient implements LookupRpcClient
type lookupRpcClient struct {
	serviceID string
	clientID  string
}

// NewLookupRpcClient constructs a new LookupRpcClient directive.
func NewLookupRpcClient(serviceID, clientID string) LookupRpcClient {
	return &lookupRpcClient{serviceID: serviceID, clientID: clientID}
}

// ExLookupRpcClient executes the LookupRpcClient directive.
// Returns if the directive becomes idle (most likely: service not found).
// If no values are returned, returns nil, nil, nil
// If values are returned, returns vals, valsRef, nil
// Otherwise returns nil, nil, err
// If waitOne is set, waits for at least one value before returning.
// disposeCb is called if the directive or any of the clients is released.
// disposeCb is optional
func ExLookupRpcClient(
	ctx context.Context,
	b bus.Bus,
	serviceID, clientID string,
	waitOne bool,
	disposeCb func(),
) ([]LookupRpcClientValue, directive.Instance, directive.Reference, error) {
	vals, di, valsRef, err := bus.ExecCollectValues[LookupRpcClientValue](
		ctx,
		b,
		NewLookupRpcClient(serviceID, clientID),
		waitOne,
		disposeCb,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(vals) == 0 {
		valsRef.Release()
		return nil, nil, nil, nil
	}
	return vals, di, valsRef, nil
}

// ExLookupRpcClientSet executes the LookupRpcClient directive returning a ClientSet.
// Returns ErrServiceClientUnavailable if no clients are returned.
// If waitOne is set, waits for at least one value before returning.
// disposeCb is called if the directive or any of the clients is released.
// disposeCb is optional
func ExLookupRpcClientSet(
	ctx context.Context,
	b bus.Bus,
	serviceID, clientID string,
	waitOne bool,
	disposeCb func(),
) (*srpc.ClientSet, directive.Instance, directive.Reference, error) {
	clients, di, clientsRef, err := ExLookupRpcClient(ctx, b, serviceID, clientID, waitOne, disposeCb)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(clients) == 0 {
		return nil, nil, nil, ErrServiceClientUnavailable
	}
	return srpc.NewClientSet(clients), di, clientsRef, nil
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupRpcClient) Validate() error {
	if d.serviceID == "" {
		return srpc.ErrEmptyServiceID
	}

	return nil
}

// GetValueLookupRpcClientOptions returns options relating to value handling.
func (d *lookupRpcClient) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 250,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupRpcServiceID returns the ID of the service.
// Cannot be empty.
func (d *lookupRpcClient) LookupRpcServiceID() string {
	return d.serviceID
}

// LookupRpcClientID returns the identifier of the caller.
// Use this for call routing only, not authentication.
// Can be empty.
func (d *lookupRpcClient) LookupRpcClientID() string {
	return d.clientID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupRpcClient) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupRpcClient)
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
func (d *lookupRpcClient) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupRpcClient) GetName() string {
	return "LookupRpcClient"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupRpcClient) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["service-id"] = []string{d.LookupRpcServiceID()}
	if clientID := d.LookupRpcClientID(); clientID != "" {
		vals["client-id"] = []string{clientID}
	}
	return vals
}

// _ is a type assertion
var _ LookupRpcClient = ((*lookupRpcClient)(nil))

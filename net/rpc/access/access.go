package bifrost_rpc_access

import (
	b58 "github.com/mr-tron/base58/base58"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// NewLookupRpcServiceRequest constructs a new LookupRpcServiceRequest.
func NewLookupRpcServiceRequest(serviceID, serverID string) *LookupRpcServiceRequest {
	return &LookupRpcServiceRequest{
		ServerId:  serverID,
		ServiceId: serviceID,
	}
}

// RequestFromDirective converts a LookupRpcService directive to a request.
func RequestFromDirective(dir bifrost_rpc.LookupRpcService) *LookupRpcServiceRequest {
	return NewLookupRpcServiceRequest(dir.LookupRpcServiceID(), dir.LookupRpcServerID())
}

// Validate validates the LookupRpcServiceRequest.
func (r *LookupRpcServiceRequest) Validate() error {
	return r.ToDirective().Validate()
}

// ToDirective converts the request to a directive.
func (r *LookupRpcServiceRequest) ToDirective() bifrost_rpc.LookupRpcService {
	return bifrost_rpc.NewLookupRpcService(r.GetServiceId(), r.GetServerId())
}

// MarshalComponentID marshals the b58 component ID.
func (r *LookupRpcServiceRequest) MarshalComponentID() (string, error) {
	data, err := r.MarshalVT()
	if err != nil {
		return "", err
	}
	return b58.Encode(data), nil
}

// UnmarshalComponentID unmarshals the component ID.
func (r *LookupRpcServiceRequest) UnmarshalComponentID(componentID string) error {
	data, err := b58.Decode(componentID)
	if err != nil {
		return err
	}
	return r.UnmarshalVT(data)
}

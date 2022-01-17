package identity_domain

import (
	"github.com/aperturerobotics/bifrost/protocol"
	"github.com/golang/protobuf/proto"
)

// IdentityDomainProtocol is the identity domain lookup service protocol.
const IdentityDomainProtocol = protocol.ID("aperture-identity/domain")

// UnmarshalFrom unmarshals the request from data.
func (r *LookupEntityReq) UnmarshalFrom(data []byte) error {
	return proto.Unmarshal(data, r)
}

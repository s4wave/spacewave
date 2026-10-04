package resource_server

import "github.com/aperturerobotics/starpc/srpc"

// resourceRPCStream transfers registered resource IDs on successful sends.
type resourceRPCStream struct {
	// Stream carries the invocation and transport.
	srpc.Stream
	// resource tracks unpublished registrations for this invocation.
	resource *resourceRPCContext
}

// MsgSend publishes the registrations preceding this response on success.
func (s *resourceRPCStream) MsgSend(msg srpc.Message) error {
	return s.resource.send(s.Stream, msg)
}

// _ is a type assertion
var _ srpc.Stream = (*resourceRPCStream)(nil)

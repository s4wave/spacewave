//go:build !tinygo

package resource_root

import (
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// webBindingStream checks each request message it receives and narrows each
// response it sends.
type webBindingStream struct {
	// Stream carries the method's request and response messages.
	srpc.Stream
	// binding identifies the scope checked by the selected policies.
	binding *webBinding
	// method identifies denied requests in logs and errors.
	method string
	// check admits request messages, or is nil for unrestricted requests.
	check webBindingCheck
	// narrow projects response messages, or is nil for unchanged responses.
	narrow func(b *webBinding, msg srpc.Message)
}

// MsgRecv receives a request message and rejects it unless the check admits it.
func (s *webBindingStream) MsgRecv(msg srpc.Message) error {
	// Receive the request before applying its scope check.
	if err := s.Stream.MsgRecv(msg); err != nil {
		return err
	}

	// Reject requests outside the configured listener scope.
	if s.check != nil && !s.check(s.binding, msg) {
		s.binding.le.WithField("method", s.method).Info("web binding denied request")
		return errors.Wrap(errWebBindingDenied, s.method)
	}

	// Admit the checked request.
	return nil
}

// MsgSend narrows a response message and sends it.
func (s *webBindingStream) MsgSend(msg srpc.Message) error {
	// Project the response before passing it to the transport.
	if s.narrow != nil {
		s.narrow(s.binding, msg)
	}

	// Send the projected response through the underlying stream.
	return s.Stream.MsgSend(msg)
}

var _ srpc.Stream = (*webBindingStream)(nil)

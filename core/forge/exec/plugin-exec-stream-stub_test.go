package space_exec

import (
	"context"
	"io"

	"github.com/aperturerobotics/starpc/srpc"
)

// pluginExecStreamStub supplies queued responses or a test-controlled stream.
type pluginExecStreamStub struct {
	// resps supplies responses when no channel is configured.
	resps []*PluginExecResponse
	// ch supplies responses under test-controlled synchronization.
	ch chan *PluginExecResponse
	// idx is the next queued response, advanced by Recv.
	idx int
}

// Context supplies the stub stream lifecycle.
func (s *pluginExecStreamStub) Context() context.Context {
	return context.Background()
}

// MsgSend accepts unused generic messages.
func (s *pluginExecStreamStub) MsgSend(srpc.Message) error {
	return nil
}

// MsgRecv accepts unused generic reads.
func (s *pluginExecStreamStub) MsgRecv(srpc.Message) error {
	return nil
}

// CloseSend has no outgoing transport to close.
func (s *pluginExecStreamStub) CloseSend() error {
	return nil
}

// Close has no retained transport to release.
func (s *pluginExecStreamStub) Close() error {
	return nil
}

// Recv delivers the next response and reports EOF after the source ends.
func (s *pluginExecStreamStub) Recv() (*PluginExecResponse, error) {
	// Receive responses under the test's stream gate when configured.
	if s.ch != nil {
		resp, ok := <-s.ch
		if !ok {
			return nil, io.EOF
		}
		return resp, nil
	}

	// Advance through the queued plugin responses.
	if s.idx >= len(s.resps) {
		return nil, io.EOF
	}
	resp := s.resps[s.idx]
	s.idx++
	return resp, nil
}

// RecvTo decodes the next configured response into the caller's message.
func (s *pluginExecStreamStub) RecvTo(resp *PluginExecResponse) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	*resp = *next
	return nil
}

// _ verifies the production plugin stream contract.
var _ SRPCPluginExecService_ExecuteStreamClient = (*pluginExecStreamStub)(nil)

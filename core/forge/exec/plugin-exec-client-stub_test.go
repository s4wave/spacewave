package space_exec

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
)

// pluginExecClientStub returns configured plugin responses and records calls.
type pluginExecClientStub struct {
	// req is the most recent request, read after call completion or streamStarted.
	req *PluginExecRequest
	// resp is the configured unary response.
	resp *PluginExecResponse
	// err is the configured unary failure.
	err error
	// stream supplies the configured stream responses.
	stream *pluginExecStreamStub
	// streamErr rejects opening the stream when configured.
	streamErr error
	// streamCalled records whether the streaming interface was used.
	streamCalled bool
	// streamStarted closes after the stream request has been recorded.
	streamStarted chan struct{}
}

// SRPCClient reports that this stub has no attached Resource transport.
func (s *pluginExecClientStub) SRPCClient() srpc.Client {
	return nil
}

// Execute captures the request and returns the configured unary response.
func (s *pluginExecClientStub) Execute(
	ctx context.Context,
	req *PluginExecRequest,
) (*PluginExecResponse, error) {
	s.req = req
	return s.resp, s.err
}

// ExecuteStream captures the request and signals that the configured stream opened.
func (s *pluginExecClientStub) ExecuteStream(
	ctx context.Context,
	req *PluginExecRequest,
) (SRPCPluginExecService_ExecuteStreamClient, error) {
	// Record the request before signaling that the plugin stream has opened.
	s.req = req
	s.streamCalled = true
	if s.streamStarted != nil {
		close(s.streamStarted)
	}

	// Return the configured plugin stream or opening failure.
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	return s.stream, nil
}

// _ verifies the production plugin client contract.
var _ SRPCPluginExecServiceClient = (*pluginExecClientStub)(nil)

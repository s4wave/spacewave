package space_exec

import "context"

// executionContextService captures the request received by a plugin over RPC.
type executionContextService struct {
	// requests delivers decoded requests after the plugin receives them.
	requests chan *PluginExecRequest
}

// Execute records the caller's context without reading its Execution body.
func (s *executionContextService) Execute(ctx context.Context, req *PluginExecRequest) (*PluginExecResponse, error) {
	s.requests <- req.CloneVT()
	return &PluginExecResponse{}, nil
}

// ExecuteStream sends the same successful result over the streaming interface.
func (s *executionContextService) ExecuteStream(req *PluginExecRequest, stream SRPCPluginExecService_ExecuteStreamStream) error {
	resp, err := s.Execute(stream.Context(), req)
	if err != nil {
		return err
	}
	return stream.Send(resp)
}

// _ verifies the plugin's production service contract.
var _ SRPCPluginExecServiceServer = (*executionContextService)(nil)

package s4wave_session

import (
	"context"
	"io"

	"github.com/pkg/errors"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/net/peer"
	stream_api "github.com/s4wave/spacewave/net/stream/api"
	stream_api_accept "github.com/s4wave/spacewave/net/stream/api/accept"
	stream_api_dial "github.com/s4wave/spacewave/net/stream/api/dial"
	stream_api_rpc "github.com/s4wave/spacewave/net/stream/api/rpc"
)

// PeerTransport provides authenticated stream access for a local session.
// The transport resource reference is owned by the PeerTransport and must be
// released when the transport is no longer needed.
type PeerTransport struct {
	// ref retains the mounted stream service until Release.
	ref resource_client.ResourceRef
	// service opens account-authenticated peer streams.
	service stream_api.SRPCStreamServiceClient
	// localID is the decoded identity used by stream connections.
	localID peer.ID

	// PeerID is the authenticated local account session peer ID.
	PeerID string
}

// OpenPeerTransport opens the authenticated stream service for this session.
func (s *Session) OpenPeerTransport(ctx context.Context) (*PeerTransport, error) {
	resp, err := s.service.AccessPeerTransport(ctx, &AccessPeerTransportRequest{})
	if err != nil {
		return nil, err
	}

	ref := s.client.CreateResourceReference(resp.GetResourceId())
	srpcClient, err := ref.GetClient()
	if err != nil {
		ref.Release()
		return nil, err
	}
	localID, err := peer.IDB58Decode(resp.GetPeerId())
	if err != nil {
		ref.Release()
		return nil, errors.Wrap(err, "parse local peer ID")
	}

	return &PeerTransport{
		ref:     ref,
		service: stream_api.NewSRPCStreamServiceClient(srpcClient),
		localID: localID,
		PeerID:  localID.String(),
	}, nil
}

// Release releases the authenticated stream service resource.
func (p *PeerTransport) Release() {
	p.ref.Release()
}

// Dial opens a stream to the exact remote peer using protocolID.
func (p *PeerTransport) Dial(ctx context.Context, remotePeerID, protocolID string) (io.ReadWriteCloser, error) {
	remoteID, err := peer.IDB58Decode(remotePeerID)
	if err != nil {
		return nil, err
	}
	conf := &stream_api_dial.Config{
		PeerId:      remoteID.String(),
		LocalPeerId: p.PeerID,
		ProtocolId:  protocolID,
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}

	stream, err := p.service.DialStream(ctx)
	if err != nil {
		return nil, err
	}
	keepStream := false
	defer func() {
		if !keepStream {
			_ = stream.Close()
		}
	}()

	if err := stream.Send(&stream_api.DialStreamRequest{Config: conf}); err != nil {
		return nil, err
	}
	rpc := stream_api.NewDialStreamClientRPC(stream)
	if err := waitForEstablished(rpc); err != nil {
		return nil, err
	}
	keepStream = true
	return stream_api_rpc.NewNetConn(p.localID, remoteID, rpc), nil
}

// Accept accepts a stream from the exact remote peer using protocolID.
func (p *PeerTransport) Accept(ctx context.Context, remotePeerID, protocolID string) (io.ReadWriteCloser, error) {
	remoteID, err := peer.IDB58Decode(remotePeerID)
	if err != nil {
		return nil, err
	}
	conf := &stream_api_accept.Config{
		LocalPeerId:   p.PeerID,
		RemotePeerIds: []string{remoteID.String()},
		ProtocolId:    protocolID,
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}

	stream, err := p.service.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	keepStream := false
	defer func() {
		if !keepStream {
			_ = stream.Close()
		}
	}()

	if err := stream.Send(&stream_api.AcceptStreamRequest{Config: conf}); err != nil {
		return nil, err
	}
	rpc := stream_api.NewAcceptStreamClientRPC(stream)
	if err := waitForEstablished(rpc); err != nil {
		return nil, err
	}
	keepStream = true
	return stream_api_rpc.NewNetConn(p.localID, remoteID, rpc), nil
}

// waitForEstablished consumes connection states until the peer stream is ready.
func waitForEstablished(rpc stream_api_rpc.RPC) error {
	for {
		data, err := rpc.Recv()
		if err != nil {
			return err
		}
		if data.GetState() == stream_api_rpc.StreamState_StreamState_ESTABLISHED {
			return nil
		}
	}
}

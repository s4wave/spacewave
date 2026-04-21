package stream_srpc

import (
	"context"
	"errors"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/sirupsen/logrus"
)

// NewOpenStreamFunc constructs a new OpenStreamFunc which establishes a
// connection with the given peer on-demand when a RPC call is made.
//
// transportID and srcPeer are optional
// starts a read pump in a goroutine
func NewOpenStreamFunc(
	b bus.Bus,
	protocolID protocol.ID,
	srcPeer, destPeer peer.ID,
	transportID uint64,
) srpc.OpenStreamFunc {
	return func(
		ctx context.Context,
		msgHandler srpc.PacketDataHandler,
		closeHandler srpc.CloseHandler,
	) (srpc.PacketWriter, error) {
		return EstablishSrpcStream(
			ctx,
			b,
			protocolID,
			srcPeer, destPeer,
			transportID,
			msgHandler, closeHandler,
		)
	}
}

// NewMultiOpenStreamFunc builds a func which attempts multiple peers.
//
// if timeoutDur <= 0, uses no timeout.
func NewMultiOpenStreamFunc(
	b bus.Bus,
	le *logrus.Entry,
	protocolID protocol.ID,
	srcPeer peer.ID, destPeers []peer.ID,
	transportID uint64,
	timeoutDur time.Duration,
) srpc.OpenStreamFunc {
	return func(
		ctx context.Context,
		msgHandler srpc.PacketDataHandler,
		closeHandler srpc.CloseHandler,
	) (srpc.PacketWriter, error) {
		var lastErr error
		for _, destPeer := range destPeers {
			var estCtx context.Context
			var estCtxCancel context.CancelFunc
			if timeoutDur > 0 {
				estCtx, estCtxCancel = context.WithTimeout(ctx, timeoutDur)
			} else {
				estCtx, estCtxCancel = context.WithCancel(ctx)
			}

			le := le.WithField("server-peer-id", destPeer.String())
			writer, err := EstablishSrpcStream(
				estCtx,
				b,
				protocolID,
				srcPeer, destPeer,
				transportID,
				msgHandler,
				closeHandler,
			)
			estCtxCancel()
			if err != nil {
				// detect deadline exceeded
				if err == context.Canceled && estCtx.Err() != nil && ctx.Err() == nil {
					err = context.DeadlineExceeded
				}
				le.WithError(err).Warn("unable to establish srpc conn")
				lastErr = err
				continue
			}
			return writer, nil
		}

		if lastErr == nil {
			lastErr = errors.New("connection failed")
		}

		return nil, lastErr
	}
}

// EstablishSrpcStream establishes a srpc stream via a Bifrost stream.
//
// transportID and srcPeer are optional
// starts a read pump in a goroutine
func EstablishSrpcStream(
	ctx context.Context,
	b bus.Bus,
	protocolID protocol.ID,
	srcPeer, destPeer peer.ID,
	transportID uint64,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
) (srpc.PacketWriter, error) {
	ms, msRel, err := link.OpenStreamWithPeerEx(
		ctx,
		b,
		protocolID,
		srcPeer, destPeer, transportID,
		stream.OpenOpts{},
	)
	if err != nil {
		return nil, err
	}

	rw := srpc.NewPacketReadWriter(ms.GetStream())
	go func() {
		rw.ReadPump(msgHandler, closeHandler)
		msRel()
	}()

	return rw, nil
}

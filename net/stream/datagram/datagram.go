// Package stream_datagram forwards UDP packets over an established peer
// message stream. Each stream has one fixed UDP peer and one socket; peer
// authentication and stream admission belong to the caller's existing
// link/session authority.
package stream_datagram

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"

	"github.com/s4wave/spacewave/net/stream"
	"golang.org/x/sync/errgroup"
)

// MaxPacketSize bounds packets to the maximum IPv4 UDP payload.
//
// Each packet is sent as one message on the peer stream, so a lost packet
// never delays the ones after it. A packet too large for one message falls
// back to the reliable control stream as a two-byte big-endian payload length
// followed by exactly that many bytes. Empty datagrams are preserved. Packets
// may arrive out of order, as UDP permits.
const MaxPacketSize = 65507

// outboxSize bounds the packets waiting for the peer stream. When the link
// sends slower than the application, the oldest waiting packet is dropped:
// a backlog would delay every later packet, and real-time traffic wants the
// newest one.
const outboxSize = 8

// Forward owns socket and peerStream until it returns, including on failure.
// socket must be unconnected and peer is the only permitted UDP sender and
// destination. Other local senders are ignored. Cancellation or either side
// failing closes both resources and waits for every forwarding pump to finish.
// The caller binds a fresh socket per player/session; never share this socket
// with another Forward call. Only admitted sessions should reach this API.
func Forward(ctx context.Context, socket *net.UDPConn, peer *net.UDPAddr, peerStream stream.MessageStream) error {
	if peer == nil || peer.IP == nil || peer.Port <= 0 || peer.Port > 65535 {
		socket.Close()
		peerStream.Close()
		return errors.New("datagram peer must have an IP and port")
	}
	return forward(ctx, socket, peer.AddrPort(), peerStream)
}

// ForwardLocal forwards one local application's UDP association. socket must
// be bound to loopback. Its first sender becomes the fixed peer for this stream;
// packets from other senders are ignored. The first packet is retained and
// forwarded. The caller owns admission of the remote stream and gives only its
// selected local application this endpoint. Cancellation closes both resources.
func ForwardLocal(ctx context.Context, socket *net.UDPConn, peerStream stream.MessageStream) error {
	if !socket.LocalAddr().(*net.UDPAddr).IP.IsLoopback() {
		socket.Close()
		peerStream.Close()
		return errors.New("local datagram endpoint must be bound to loopback")
	}
	return forward(ctx, socket, netip.AddrPort{}, peerStream)
}

// ForwardTarget allocates a dedicated upstream UDP association for one peer
// stream, then forwards to a fixed service endpoint. Separate calls produce
// separate source ports at the service, so replies cannot cross player streams.
// It owns peerStream even if binding the socket fails.
func ForwardTarget(ctx context.Context, target *net.UDPAddr, peerStream stream.MessageStream) error {
	// Select the upstream UDP network from the fixed target address.
	network := "udp4"
	if target != nil && target.IP.To4() == nil {
		network = "udp6"
	}

	// Bind a dedicated UDP association and forward the peer stream to its target.
	socket, err := net.ListenUDP(network, nil)
	if err != nil {
		peerStream.Close()
		return err
	}
	return Forward(ctx, socket, target, peerStream)
}

// forward runs the socket pump and the two peer pumps until one fails.
func forward(ctx context.Context, socket *net.UDPConn, remote netip.AddrPort, peerStream stream.MessageStream) error {
	// Close both forwarding resources on exit and require an unconnected UDP socket.
	defer socket.Close()
	defer peerStream.Close()
	if socket.RemoteAddr() != nil {
		return errors.New("datagram socket must be unconnected")
	}

	// Replies wait for the first local sender when the peer is not fixed.
	peerReady := make(chan struct{})
	if remote.IsValid() {
		close(peerReady)
	}
	group, groupCtx := errgroup.WithContext(ctx)
	stop := context.AfterFunc(groupCtx, func() { socket.Close(); peerStream.Close() })
	defer stop()

	// Read accepted UDP packets as fast as they arrive, so a slow peer stream
	// never leaves them waiting in the socket buffer.
	outbox := make(chan []byte, outboxSize)
	group.Go(func() error {
		packet := make([]byte, MaxPacketSize+1)
		for {
			n, source, err := socket.ReadFromUDPAddrPort(packet)
			if err != nil {
				return err
			}
			if !remote.IsValid() {
				if !source.Addr().IsLoopback() {
					continue
				}
				remote = source
				close(peerReady)
			}
			if source.Addr().Unmap() != remote.Addr().Unmap() || source.Port() != remote.Port() {
				continue
			}
			if n > MaxPacketSize {
				return errors.New("UDP packet exceeds maximum size")
			}
			push(outbox, bytes.Clone(packet[:n]))
		}
	})

	// Send each queued packet as one message, or framed on the control stream
	// when it does not fit in one.
	group.Go(func() error {
		control := peerStream.Control()
		for {
			var packet []byte
			select {
			case packet = <-outbox:
			case <-groupCtx.Done():
				return groupCtx.Err()
			}
			if _, err := peerStream.Write(packet); err == nil {
				continue
			}
			if err := writeFrame(control, packet); err != nil {
				return err
			}
		}
	})

	// Deliver received messages and control frames to the UDP peer.
	deliver := func(read func([]byte) (int, error)) func() error {
		return func() error {
			select {
			case <-peerReady:
			case <-groupCtx.Done():
				return groupCtx.Err()
			}
			packet := make([]byte, MaxPacketSize)
			for {
				n, err := read(packet)
				if err != nil {
					return err
				}
				if _, err := socket.WriteToUDPAddrPort(packet[:n], remote); err != nil {
					return err
				}
			}
		}
	}
	group.Go(deliver(peerStream.Read))
	group.Go(deliver(func(packet []byte) (int, error) {
		return readFrame(peerStream.Control(), packet)
	}))

	// Join every forwarding pump and preserve caller cancellation as the result.
	err := group.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// push queues packet, dropping the oldest waiting packet when queue is full.
// It needs to be the queue's only sender.
func push(queue chan []byte, packet []byte) {
	for {
		select {
		case queue <- packet:
			return
		default:
		}
		select {
		case <-queue:
		default:
		}
	}
}

// writeFrame writes packet to w as one length-prefixed frame.
func writeFrame(w io.Writer, packet []byte) error {
	// Encode and write the packet with its control-stream length prefix.
	frame := make([]byte, 2+len(packet))
	binary.BigEndian.PutUint16(frame, uint16(len(packet))) //nolint:gosec // callers bound packets by MaxPacketSize.
	copy(frame[2:], packet)
	_, err := w.Write(frame)
	return err
}

// readFrame reads one length-prefixed frame from r into packet, which holds
// at least MaxPacketSize bytes.
func readFrame(r io.Reader, packet []byte) (int, error) {
	// Read the control frame length before accepting its packet payload.
	var header [2]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, err
	}

	// Reject control frames larger than the supported UDP payload.
	n := int(binary.BigEndian.Uint16(header[:]))
	if n > MaxPacketSize {
		return 0, errors.New("stream packet exceeds maximum size")
	}
	return io.ReadFull(r, packet[:n])
}

// Package stream_datagram forwards UDP packets over an established peer stream.
// Each stream has one fixed UDP peer and one socket; peer authentication and
// stream admission belong to the caller's existing link/session authority.
package stream_datagram

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"

	"golang.org/x/sync/errgroup"
)

// MaxPacketSize bounds packets to the maximum IPv4 UDP payload. The wire format
// is a two-byte big-endian payload length followed by exactly that many bytes.
// Empty datagrams are preserved. A reliable stream retains ordering and can
// introduce head-of-line blocking; this adapter does not change that transport.
const MaxPacketSize = 65507

// Forward owns socket and peerStream until it returns, including on failure.
// socket must be unconnected and peer is the only permitted UDP sender and
// destination. Other local senders are ignored. Cancellation or either side
// failing closes both resources and waits for both forwarding pumps to finish.
// The caller binds a fresh socket per player/session; never share this socket
// with another Forward call. Only admitted sessions should reach this API.
func Forward(ctx context.Context, socket *net.UDPConn, peer *net.UDPAddr, peerStream io.ReadWriteCloser) error {
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
func ForwardLocal(ctx context.Context, socket *net.UDPConn, peerStream io.ReadWriteCloser) error {
	if !socket.LocalAddr().(*net.UDPAddr).IP.IsLoopback() {
		socket.Close()
		peerStream.Close()
		return errors.New("local datagram endpoint must be bound to loopback")
	}
	return forward(ctx, socket, netip.AddrPort{}, peerStream)
}

func forward(ctx context.Context, socket *net.UDPConn, remote netip.AddrPort, peerStream io.ReadWriteCloser) error {
	defer socket.Close()
	defer peerStream.Close()
	if socket.RemoteAddr() != nil {
		return errors.New("datagram socket must be unconnected")
	}
	peerReady := make(chan struct{})
	if remote.IsValid() {
		close(peerReady)
	}
	group, groupCtx := errgroup.WithContext(ctx)
	stop := context.AfterFunc(groupCtx, func() { socket.Close(); peerStream.Close() })
	defer stop()
	group.Go(func() error {
		packet := make([]byte, MaxPacketSize+1)
		var header [2]byte
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
			binary.BigEndian.PutUint16(header[:], uint16(n))
			if err := writeFull(peerStream, header[:]); err != nil {
				return err
			}
			if err := writeFull(peerStream, packet[:n]); err != nil {
				return err
			}
		}
	})
	group.Go(func() error {
		select {
		case <-peerReady:
		case <-groupCtx.Done():
			return groupCtx.Err()
		}
		packet := make([]byte, MaxPacketSize)
		var header [2]byte
		for {
			if _, err := io.ReadFull(peerStream, header[:]); err != nil {
				return err
			}
			n := int(binary.BigEndian.Uint16(header[:]))
			if n > MaxPacketSize {
				return errors.New("stream packet exceeds maximum size")
			}
			if _, err := io.ReadFull(peerStream, packet[:n]); err != nil {
				return err
			}
			if _, err := socket.WriteToUDPAddrPort(packet[:n], remote); err != nil {
				return err
			}
		}
	})
	err := group.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// ForwardTarget allocates a dedicated upstream UDP association for one peer
// stream, then forwards to a fixed service endpoint. Separate calls produce
// separate source ports at the service, so replies cannot cross player streams.
// It owns peerStream even if binding the socket fails.
func ForwardTarget(ctx context.Context, target *net.UDPAddr, peerStream io.ReadWriteCloser) error {
	network := "udp4"
	if target != nil && target.IP.To4() == nil {
		network = "udp6"
	}
	socket, err := net.ListenUDP(network, nil)
	if err != nil {
		peerStream.Close()
		return err
	}
	return Forward(ctx, socket, target, peerStream)
}

func writeFull(writer io.Writer, data []byte) error {
	for len(data) != 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

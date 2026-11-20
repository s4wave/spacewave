//go:build !js

package checkoutroot

import (
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_server "github.com/s4wave/spacewave/db/unixfs/rpc/server"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/sirupsen/logrus"
)

// Server serves one placed checkout root for a controller execution.
type Server struct {
	// ctx ends active connections when the node stops.
	ctx context.Context
	// le reports failures opening an incoming stream.
	le *logrus.Entry
	// b resolves the Session transport and retains the peer link.
	b bus.Bus
	// protocol identifies the stream for this folder and access mode.
	protocol protocol.ID
	// root supplies independent cursors that share the same filesystem.
	root *unixfs_billy.BillyFSCursor
	// watcher routes host changes to the open cursors of this root.
	watcher *DirectoryWatcher
	// upload publishes files when the node allows writes.
	upload unixfs_rpc_server.FileUpload
}

// serve adds the handler until the checkout-root controller stops.
func serve(ctx context.Context, le *logrus.Entry, b bus.Bus, id protocol.ID, folder *os.Root, root *unixfs_billy.BillyFSCursor, upload unixfs_rpc_server.FileUpload) error {
	// Keep one watcher for the root and end its connections if the watcher stops.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	watcher, err := NewDirectoryWatcher(folder)
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Offer the checkout root to the Device's Session transport.
	s := &Server{ctx: ctx, le: le, b: b, protocol: id, root: root, watcher: watcher, upload: upload}
	release, err := b.AddHandler(s)
	if err != nil {
		return err
	}
	defer release()

	// Dispatch host events while the handler is published.
	return watcher.Run(ctx, le)
}

// HandleDirective offers this root when the incoming stream names its protocol.
func (s *Server) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	// Match the placement and access mode identified by the incoming protocol.
	dir, ok := di.GetDirective().(link.HandleMountedStream)
	if !ok || dir.HandleMountedStreamProtocolID() != s.protocol {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]link.MountedStreamHandler{s}), nil)
}

// HandleMountedStream admits and serves one checkout-root connection asynchronously.
func (s *Server) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	// UnixFS RPCs use a reliable stream for their multiplexed connection.
	if ms.GetOpenOpts().Unreliable {
		return errors.New("checkout-root RPC requires a reliable stream")
	}

	// Leave link dispatch free to admit other streams.
	go s.serveStream(ctx, ms)
	return nil
}

// serveStream admits the peer before the RPC server reads the stream.
func (s *Server) serveStream(linkCtx context.Context, ms link.MountedStream) {
	// End the connection if its link or checkout-root node stops.
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	stopOnLinkEnd := context.AfterFunc(linkCtx, cancel)
	defer stopOnLinkEnd()

	// Track the stream for revocation and enforce the account boundary.
	conn, err := transport.AuthorizeAccountSessionPeer(ctx, s.b, ms)
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer func() {
		if stopClose() {
			_ = conn.Close()
		}
	}()
	if err != nil {
		if ctx.Err() == nil {
			s.le.WithError(err).Warn("checkout-root peer refused")
		}
		return
	}

	// Hold the link while the admitted peer has the root open.
	_, ref, err := s.b.AddDirective(ctx, link.NewEstablishLinkWithPeer(ms.GetLink().GetLocalPeer(), ms.GetPeerID()), nil)
	if err != nil {
		s.le.WithError(err).Debug("checkout-root link ended")
		return
	}
	defer ref.Release()

	// Give this connection its own set of cursor and operation handles.
	cursor := s.root.Clone()
	service := unixfs_rpc_server.NewFSCursorServiceWithDirectory(cursor, s.upload, s.watcher.Watch)
	defer service.Release(true)
	mux := srpc.NewMux()
	if err := unixfs_rpc.SRPCRegisterFSCursorService(mux, service); err != nil {
		s.le.WithError(err).Error("register checkout-root RPC")
		return
	}

	// Serve the RPC substreams until the peer closes or admission ends.
	mc, err := srpc.NewMuxedConnWithRwc(ctx, conn, false, nil)
	if err != nil {
		s.le.WithError(err).Debug("open checkout-root RPC")
		return
	}
	defer mc.Close()
	_ = srpc.NewServer(mux).AcceptMuxedConn(ctx, mc)
}

// _ is a type assertion.
var (
	_ directive.Handler         = (*Server)(nil)
	_ link.MountedStreamHandler = (*Server)(nil)
)

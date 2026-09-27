//go:build !js

// Package daemon connects native Spacewave clients to the shared daemon.
package daemon

import (
	"context"
	"net"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// Client retains one initialized Resource connection. Closing it never stops
// the daemon, including when this caller started that daemon.
type Client struct {
	// conn is the socket owned by this client.
	conn net.Conn
	// rpc carries services on the socket.
	rpc srpc.Client
	// resources retains the Resource Init stream.
	resources *resource_client.Client
	// root is the client's root Resource reference.
	root *s4wave_root.Root
	// cancel ends the Resource stream independently of its startup deadline.
	cancel context.CancelFunc
}

// NewClient initializes Resource service on conn and takes ownership of conn
// on both success and failure. The caller must Close a successful client.
func NewClient(ctx context.Context, conn net.Conn) (*Client, error) {
	// Bound initialization without imposing a deadline on the retained stream.
	timeout, err := StartupTimeout()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	clientCtx, cancel := context.WithCancel(ctx)
	c := &Client{conn: conn, cancel: cancel}
	timer := time.AfterFunc(timeout, cancel)
	defer timer.Stop()
	stopTransport := context.AfterFunc(clientCtx, func() { _ = conn.Close() })
	defer stopTransport()

	// Join initialization before releasing a canceled transport.
	c.rpc, err = srpc.NewClientWithConn(conn, true, nil)
	if err == nil {
		c.resources, err = resource_client.NewClient(clientCtx, resource.NewSRPCResourceServiceClient(c.rpc))
	}
	if err == nil {
		ref := c.resources.AccessRootResource()
		c.root, err = s4wave_root.NewRoot(c.resources, ref)
		if err != nil {
			ref.Release()
		}
	}

	// A timer already firing must finish cancellation before success is decided.
	if !timer.Stop() {
		cancel()
	}
	if err == nil {
		err = clientCtx.Err()
	}
	if err != nil {
		c.Close()
		return nil, errors.Wrap(err, "resource client init")
	}
	return c, nil
}

// Conn returns the transport for connection-scoped control operations.
func (c *Client) Conn() net.Conn { return c.conn }

// RPC returns the socket's service client.
func (c *Client) RPC() srpc.Client { return c.rpc }

// Resources returns the initialized Resource client.
func (c *Client) Resources() *resource_client.Client { return c.resources }

// Root returns the root reference retained by this client.
func (c *Client) Root() *s4wave_root.Root { return c.root }

// Close releases the root, Resource stream and socket. It never signals a process.
func (c *Client) Close() {
	// Release adopted references before the connection that serves them.
	if c.root != nil {
		c.root.Release()
	}
	if c.resources != nil {
		c.resources.Release()
	}

	// End the transport's lifetime even when initialization failed.
	c.cancel()
	_ = c.conn.Close()
}

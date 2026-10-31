//go:build !tinygo

// Package block_store_ftps stores the objects of a block_store_s3.PackStore
// on an FTP server over explicit TLS (AUTH TLS), the protocol of
// curl --ssl-reqd ftp://.
package block_store_ftps

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"io"
	"net"
	"net/textproto"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// DefaultConns is the default number of control connections of a Client.
const DefaultConns = 8

// dialTimeout bounds connecting, the TLS handshake and each data connection.
const dialTimeout = 30 * time.Second

// tempPrefix starts the name of an object being written. ListObjects skips
// such names.
const tempPrefix = ".tmp-"

// Client is a block_store_s3.ObjectStore on an FTPS server.
//
// A bucket is a directory relative to the login directory, and a key is a
// slash path below it; missing directories are created on write. Each call
// takes one control connection from a pool of Config.Conns, so that many calls
// run in parallel, and a call whose connection dropped redials once.
//
// The FTP commands do not take a context: ctx bounds the wait for a
// connection, and a started command runs until the server answers or the
// connection times out.
type Client struct {
	// conf is the configuration with Addr carrying a port.
	conf Config
	// tlsConf verifies the server and caches TLS sessions, which some servers
	// require the data connections to resume.
	tlsConf *tls.Config
	// conns holds one slot per pooled connection: a logged in connection, or
	// nil when the slot must dial before use.
	conns chan *ftp.ServerConn
}

// NewClient builds a Client. It connects lazily, on the first call.
func NewClient(conf Config) *Client {
	// Default the port and the TLS configuration.
	if _, _, err := net.SplitHostPort(conf.Addr); err != nil {
		conf.Addr = net.JoinHostPort(conf.Addr, "21")
	}
	tlsConf := &tls.Config{}
	if conf.TLSConfig != nil {
		tlsConf = conf.TLSConfig.Clone()
	}
	if tlsConf.ServerName == "" {
		tlsConf.ServerName, _, _ = net.SplitHostPort(conf.Addr)
	}
	if tlsConf.ClientSessionCache == nil {
		tlsConf.ClientSessionCache = tls.NewLRUClientSessionCache(0)
	}

	// Fill the pool with empty slots.
	n := conf.Conns
	if n <= 0 {
		n = DefaultConns
	}
	conns := make(chan *ftp.ServerConn, n)
	for range n {
		conns <- nil
	}
	return &Client{conf: conf, tlsConf: tlsConf, conns: conns}
}

// Close waits for running calls and logs out every connection. A later call
// dials again.
func (c *Client) Close() {
	for range cap(c.conns) {
		if conn := <-c.conns; conn != nil {
			_ = conn.Quit()
		}
	}
	for range cap(c.conns) {
		c.conns <- nil
	}
}

// PutObject writes the object under a temporary name in its directory and
// renames it into place, so a reader never sees it partly written. Replacing
// an existing object deletes it first where the server refuses to rename over
// it, which leaves a moment without the object.
func (c *Client) PutObject(ctx context.Context, bucket, key string, data []byte, _ string) error {
	// Name the temporary file beside the object.
	name := path.Join(bucket, key)
	dir, base := path.Split(name)
	tag := make([]byte, 8)
	_, _ = rand.Read(tag)
	tmp := dir + tempPrefix + hex.EncodeToString(tag) + "-" + base

	return c.do(ctx, func(conn *ftp.ServerConn) error {
		// Upload the temporary file, creating the directories it needs.
		err := conn.Stor(tmp, bytes.NewReader(data))
		if isReply(err) {
			makeDirs(conn, dir)
			err = conn.Stor(tmp, bytes.NewReader(data))
		}
		if err != nil {
			return errors.Wrap(err, "store "+name)
		}

		// Rename it into place, replacing an object the server will not
		// rename over.
		err = conn.Rename(tmp, name)
		if isReply(err) {
			_ = conn.Delete(name)
			err = conn.Rename(tmp, name)
		}
		if err != nil {
			_ = conn.Delete(tmp)
			return errors.Wrap(err, "rename "+name)
		}
		return nil
	})
}

// GetObject reads the whole object. Packfile readers read whole objects only
// to decode them, so the body is held in memory and the connection returns to
// the pool at once.
func (c *Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	// Download the object whole on one connection.
	name := path.Join(bucket, key)
	var data []byte
	err := c.do(ctx, func(conn *ftp.ServerConn) error {
		// Start the download.
		resp, err := conn.Retr(name)
		if err != nil {
			return notFound(err, name)
		}

		// Read it to the end and close the transfer.
		data, err = io.ReadAll(resp)
		if cerr := resp.Close(); err == nil {
			err = cerr
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// GetObjectRange reads length bytes from offset off with REST and RETR, then
// closes the data connection, which ends the transfer early. A server that
// ends the session on the early close, as ProFTPD does, costs the next call
// on that slot a new connection.
func (c *Client) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	// Read the range on one connection.
	name := path.Join(bucket, key)
	var out []byte
	err := c.do(ctx, func(conn *ftp.ServerConn) error {
		// Start the download at the offset.
		resp, err := conn.RetrFrom(name, uint64(off)) //nolint:gosec // offsets are not negative.
		if err != nil {
			return notFound(err, name)
		}

		// Read the range.
		buf := make([]byte, length)
		n, err := io.ReadFull(resp, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			_ = resp.Close()
			return err
		}
		out = buf[:n]

		// Close the transfer. Closing it early, before the object ends,
		// aborts it: some servers answer with a reply, others, such as
		// ProFTPD over TLS, end the session, which spends the connection
		// but not the range already read.
		if err := resp.Close(); err != nil && !isReply(err) {
			return errSpent
		}
		return nil
	})
	if len(out) == 0 {
		out = nil
	}
	return out, err
}

// ListObjects calls fn with the key and size of every object whose key starts
// with prefix, walking the directories depth first in name order. Temporary
// files of unfinished writes are skipped. A missing directory lists nothing.
func (c *Client) ListObjects(ctx context.Context, bucket, prefix string, fn func(key string, size int64) error) error {
	dir, _ := path.Split(prefix)
	return c.do(ctx, func(conn *ftp.ServerConn) error {
		return walk(conn, bucket, dir, prefix, fn)
	})
}

// walk lists the directory dir of bucket, calling fn for the files whose keys
// start with prefix and descending into the directories that may hold some.
func walk(conn *ftp.ServerConn, bucket, dir, prefix string, fn func(key string, size int64) error) error {
	// List the directory in name order.
	entries, err := conn.List(path.Join(bucket, dir))
	if isReply(err) {
		return nil
	}
	if err != nil {
		return err
	}
	slices.SortFunc(entries, func(a, b *ftp.Entry) int { return strings.Compare(a.Name, b.Name) })

	// Visit each file and descend into each directory under the prefix.
	for _, entry := range entries {
		name := path.Base(entry.Name)
		if name == "." || name == ".." || strings.HasPrefix(name, tempPrefix) {
			continue
		}
		key := dir + name
		switch entry.Type {
		case ftp.EntryTypeFile:
			if strings.HasPrefix(key, prefix) {
				if err := fn(key, int64(entry.Size)); err != nil { //nolint:gosec // sizes fit int64.
					return err
				}
			}
		case ftp.EntryTypeFolder:
			sub := key + "/"
			if strings.HasPrefix(sub, prefix) || strings.HasPrefix(prefix, sub) {
				if err := walk(conn, bucket, sub, prefix, fn); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// DeleteObject removes the object. A missing object counts as deleted.
func (c *Client) DeleteObject(ctx context.Context, bucket, key string) error {
	name := path.Join(bucket, key)
	return c.do(ctx, func(conn *ftp.ServerConn) error {
		if err := conn.Delete(name); err != nil && !isReply(err) {
			return err
		}
		return nil
	})
}

// errSpent is returned by a do callback that finished its work but left the
// connection unusable.
var errSpent = errors.New("ftps connection spent")

// do runs fn on a pooled connection. When fn fails on the connection itself,
// rather than with a server reply, it runs once more on a fresh connection,
// since an idle connection the server closed fails only on first use. When fn
// returns errSpent, the connection is dropped and do returns nil.
func (c *Client) do(ctx context.Context, fn func(conn *ftp.ServerConn) error) error {
	// Take a connection from the pool.
	conn, err := c.acquire(ctx)
	if err != nil {
		return err
	}

	// Retry on a fresh connection when the pooled one failed.
	err = fn(conn)
	if err != nil && err != errSpent && !isReply(err) {
		_ = conn.Quit()
		conn, err = c.dial(ctx)
		if err == nil {
			err = fn(conn)
		}
	}

	// Return the slot, dropping a failed or spent connection.
	c.release(conn, err)
	if err == errSpent {
		return nil
	}
	return err
}

// acquire takes a pool slot, dialing when it holds no connection. On error
// the slot is already back in the pool.
func (c *Client) acquire(ctx context.Context) (*ftp.ServerConn, error) {
	// Wait for a free slot.
	var conn *ftp.ServerConn
	select {
	case conn = <-c.conns:
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
	if conn != nil {
		return conn, nil
	}

	// Dial the empty slot.
	conn, err := c.dial(ctx)
	if err != nil {
		c.conns <- nil
		return nil, err
	}
	return conn, nil
}

// release returns a slot to the pool, keeping conn unless err shows the
// connection failed.
func (c *Client) release(conn *ftp.ServerConn, err error) {
	if conn != nil && err != nil && !isReply(err) {
		_ = conn.Quit()
		conn = nil
	}
	c.conns <- conn
}

// dial connects, upgrades to TLS with AUTH TLS, and logs in.
func (c *Client) dial(ctx context.Context) (*ftp.ServerConn, error) {
	// Connect and upgrade the control connection to TLS.
	conn, err := ftp.Dial(c.conf.Addr,
		ftp.DialWithContext(ctx),
		ftp.DialWithTimeout(dialTimeout),
		ftp.DialWithShutTimeout(dialTimeout),
		ftp.DialWithExplicitTLS(c.tlsConf),
	)
	if err != nil {
		return nil, errors.Wrap(err, "connect to "+c.conf.Addr)
	}

	// Log in. The error names the server, never the credentials.
	if err := conn.Login(c.conf.User, c.conf.Password); err != nil {
		_ = conn.Quit()
		return nil, errors.Wrap(err, "log in to "+c.conf.Addr)
	}
	return conn, nil
}

// makeDirs creates dir and each missing parent. The errors of directories
// that exist already are ignored; the retried write reports a real failure.
func makeDirs(conn *ftp.ServerConn, dir string) {
	dir = strings.TrimSuffix(dir, "/")
	for i := range len(dir) {
		if dir[i] == '/' && i > 0 {
			_ = conn.MakeDir(dir[:i])
		}
	}
	if dir != "" {
		_ = conn.MakeDir(dir)
	}
}

// isReply reports whether err is a reply of the server, which leaves the
// control connection usable, rather than a failure of the connection.
func isReply(err error) bool {
	var reply *textproto.Error
	return errors.As(err, &reply)
}

// notFound marks a permanent negative reply to a download as
// block_store_s3.ErrNotFound.
func notFound(err error, name string) error {
	var reply *textproto.Error
	if errors.As(err, &reply) && reply.Code == ftp.StatusFileUnavailable {
		return errors.Wrap(block_store_s3.ErrNotFound, name)
	}
	return err
}

// _ is a type assertion
var _ block_store_s3.ObjectStore = (*Client)(nil)

//go:build !tinygo

// Package block_store_ftp stores blocks as files on an FTP server reached over
// explicit TLS (FTPS).
//
// The store only ever adds files: a write stores each block once under a name
// derived from its hash, and no operation deletes or rewrites a block. A
// block's file appears whole or not at all, since each write stores a
// temporary file and renames it into place.
package block_store_ftp

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/textproto"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/jlaffaye/ftp"
	"github.com/pkg/errors"
)

// defaultMaxConns is the open connection bound when Config sets none.
const defaultMaxConns = 8

// opTimeout bounds one operation, including its data transfer.
const opTimeout = 2 * time.Minute

// dialTimeout bounds opening a connection.
const dialTimeout = 10 * time.Second

// maxIdle is how long an idle connection is kept. Hosted servers drop idle
// sessions after a few minutes; a connection idle longer is closed rather than
// reused, which saves the failed command that would find it dropped.
const maxIdle = time.Minute

// tmpPrefix starts the name of a file being written. A crash can leave one
// behind; it is never read as a block.
const tmpPrefix = ".tmp-"

// Client runs file operations on a pool of FTPS connections. It is safe for
// concurrent use.
type Client struct {
	conf *Config
	addr string
	tls  *tls.Config
	// slots holds one token per open connection, bounding them.
	slots chan struct{}
	// idle holds the open connections ready for a command.
	idle chan *conn
	// dirs records the directories a file was stored into, which exist.
	dirs sync.Map
}

// NewClient builds a client of the server. It connects lazily, so an
// unreachable server fails the first operation instead.
func NewClient(conf *Config) *Client {
	// Default the port, the connection bound and the TLS server name.
	port := conf.Port
	if port == 0 {
		port = 21
	}
	maxConns := conf.MaxConns
	if maxConns <= 0 {
		maxConns = defaultMaxConns
	}
	tlsConf := conf.TLSConfig.Clone()
	if tlsConf == nil {
		tlsConf = &tls.Config{}
	}
	if tlsConf.ServerName == "" {
		tlsConf.ServerName = conf.Host
	}
	return &Client{
		conf:  conf,
		addr:  net.JoinHostPort(conf.Host, strconv.FormatUint(uint64(port), 10)),
		tls:   tlsConf,
		slots: make(chan struct{}, maxConns),
		idle:  make(chan *conn, maxConns),
	}
}

// MaxConns returns the open connection bound.
func (c *Client) MaxConns() int {
	return cap(c.slots)
}

// Close closes the idle connections. Operations still running close theirs
// as they finish.
func (c *Client) Close() {
	for {
		select {
		case cn := <-c.idle:
			c.discard(cn)
		default:
			return
		}
	}
}

// Put stores data under name, relative to the root, making its directory when
// missing. The file appears whole or not at all: data goes to a temporary
// file in the same directory, which is then renamed to name. A file already
// under name of the same length is kept, since a name derived from a hash
// holds the same bytes.
func (c *Client) Put(ctx context.Context, name string, data []byte) error {
	dir, target := c.path(path.Dir(name)), c.path(name)
	return c.do(ctx, func(sc *ftp.ServerConn) error {
		// Store a temporary file in the directory, then rename it into place.
		if err := c.makeDirs(sc, dir); err != nil {
			return err
		}
		tmp := path.Join(dir, tmpPrefix+ulid.NewULID())
		if err := sc.Stor(tmp, bytes.NewReader(data)); err != nil {
			return errors.Wrap(err, "store "+name)
		}
		if err := sc.Rename(tmp, target); err != nil {
			// A server that refuses to overwrite fails the rename when the
			// file exists; keep that file and remove the temporary one.
			_ = sc.Delete(tmp)
			if size, serr := sc.FileSize(target); serr == nil && size == int64(len(data)) {
				return nil
			}
			return errors.Wrap(err, "rename "+name)
		}
		c.dirs.Store(dir, struct{}{})
		return nil
	})
}

// Get reads the file under name. Returns false if the server reports it
// missing.
func (c *Client) Get(ctx context.Context, name string) ([]byte, bool, error) {
	// Fetch the whole file over a data connection.
	var data []byte
	err := c.do(ctx, func(sc *ftp.ServerConn) error {
		// Read the file and close the data connection.
		resp, err := sc.Retr(c.path(name))
		if err != nil {
			return err
		}
		data, err = io.ReadAll(resp)
		if cerr := resp.Close(); err == nil {
			err = cerr
		}
		return err
	})
	if isNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.Wrap(err, "read "+name)
	}
	return data, true, nil
}

// Size returns the size of the file under name. Returns false if the server
// reports it missing.
func (c *Client) Size(ctx context.Context, name string) (int64, bool, error) {
	// Ask the size on the control connection.
	var size int64
	err := c.do(ctx, func(sc *ftp.ServerConn) error {
		var err error
		size, err = sc.FileSize(c.path(name))
		return err
	})
	if isNotFound(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, errors.Wrap(err, "size "+name)
	}
	return size, true, nil
}

// path returns the server path of name under the root.
func (c *Client) path(name string) string {
	return path.Join(c.conf.Root, name)
}

// makeDirs makes dir and its parents, skipping those a file was stored into.
// A failure to make one is ignored, since it usually exists; a directory that
// is truly missing fails the store that follows.
func (c *Client) makeDirs(sc *ftp.ServerConn, dir string) error {
	if _, ok := c.dirs.Load(dir); ok {
		return nil
	}
	for i := 1; i <= len(dir); i++ {
		if i != len(dir) && dir[i] != '/' {
			continue
		}
		parent := dir[:i]
		if _, ok := c.dirs.Load(parent); ok || parent == "/" || parent == "." {
			continue
		}
		if err := sc.MakeDir(parent); err != nil && !isReply(err) {
			return err
		}
	}
	return nil
}

// do runs fn on a pooled connection within the context and opTimeout. A
// failure below the FTP protocol closes the connection, and fn runs once more
// on a new one when the failed one came from the idle pool, since the server
// may have dropped it. fn must be safe to repeat.
func (c *Client) do(ctx context.Context, fn func(*ftp.ServerConn) error) error {
	for attempt := 0; ; attempt++ {
		cn, reused, err := c.acquire(ctx)
		if err != nil {
			return err
		}
		err = cn.run(ctx, fn)
		if err == nil || (isReply(err) && !isClosing(err)) {
			c.release(cn)
			return err
		}
		c.discard(cn)
		if !reused || attempt != 0 || ctx.Err() != nil {
			return err
		}
	}
}

// acquire returns an idle connection, or opens one when the bound allows,
// waiting for either. reused reports a connection from the idle pool.
func (c *Client) acquire(ctx context.Context) (cn *conn, reused bool, err error) {
	for {
		// Prefer an idle connection over opening another.
		select {
		case cn = <-c.idle:
		default:
			select {
			case cn = <-c.idle:
			case c.slots <- struct{}{}:
				cn, err = c.dial(ctx)
				if err != nil {
					<-c.slots
					return nil, false, err
				}
				return cn, false, nil
			case <-ctx.Done():
				return nil, false, context.Cause(ctx)
			}
		}
		if time.Since(cn.used) < maxIdle {
			return cn, true, nil
		}
		c.discard(cn)
	}
}

// release returns a healthy connection to the idle pool.
func (c *Client) release(cn *conn) {
	cn.used = time.Now()
	c.idle <- cn
}

// discard closes a connection and frees its slot.
func (c *Client) discard(cn *conn) {
	cn.setDeadline(time.Now().Add(5 * time.Second))
	_ = cn.sc.Quit()
	<-c.slots
}

// dial opens and logs in a connection.
//
// ProFTPD and other servers require each data connection to resume the TLS
// session of its control connection, so each connection keeps its own session
// cache. The FTP library hands data connections from a custom dial function
// back as dialed, so dialFunc wraps every connection after the first, the
// control connection, in TLS itself.
//
// ctx bounds the control connection's dial alone. The data connections are
// dialed by later operations, which bound them through the deadline.
func (c *Client) dial(ctx context.Context) (*conn, error) {
	// Dial the control connection under ctx, and each data connection in TLS.
	cn := &conn{}
	tlsConf := c.tls.Clone()
	tlsConf.ClientSessionCache = tls.NewLRUClientSessionCache(4)
	dialer := &net.Dialer{Timeout: dialTimeout}
	dialCtx := ctx
	dialFunc := func(network, addr string) (net.Conn, error) {
		// Dial the network connection and record it under the deadline.
		nc, err := dialer.DialContext(dialCtx, network, addr)
		dialCtx = context.Background()
		if err != nil {
			return nil, err
		}
		if cn.add(nc) {
			return tls.Client(nc, tlsConf), nil
		}
		return nc, nil
	}

	// Bound the greeting, the TLS upgrade and the login like an operation.
	cn.setDeadline(time.Now().Add(opTimeout))
	sc, err := ftp.Dial(c.addr, ftp.DialWithDialFunc(dialFunc), ftp.DialWithExplicitTLS(tlsConf))
	if err != nil {
		return nil, errors.Wrap(err, "connect")
	}
	cn.sc = sc
	if err := sc.Login(c.conf.User, c.conf.Password); err != nil {
		cn.setDeadline(time.Now().Add(5 * time.Second))
		_ = sc.Quit()
		return nil, errors.Wrap(err, "log in")
	}
	return cn, nil
}

// conn is one logged-in session with the network connections it dialed.
type conn struct {
	sc *ftp.ServerConn
	// used is when the connection last returned to the idle pool.
	used time.Time

	// mtx guards the fields below.
	mtx sync.Mutex
	// control is the control connection and data the latest data connection.
	control, data net.Conn
	// deadline bounds the current operation on every network connection.
	deadline time.Time
}

// run runs fn within the context and opTimeout. Cancelling the context
// expires the deadline, which fails the blocked read or write.
func (cn *conn) run(ctx context.Context, fn func(*ftp.ServerConn) error) error {
	// Expire the deadline at opTimeout, or sooner when the context ends.
	deadline := time.Now().Add(opTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	cn.setDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { cn.setDeadline(time.Unix(1, 0)) })
	defer stop()
	return fn(cn.sc)
}

// add records a dialed network connection under the current deadline.
// Returns true for a data connection, every one after the first.
func (cn *conn) add(nc net.Conn) bool {
	// Apply the current deadline and file the connection by its order.
	cn.mtx.Lock()
	defer cn.mtx.Unlock()
	_ = nc.SetDeadline(cn.deadline)
	if cn.control == nil {
		cn.control = nc
		return false
	}
	cn.data = nc
	return true
}

// setDeadline sets the deadline of the connection's network connections.
func (cn *conn) setDeadline(t time.Time) {
	cn.mtx.Lock()
	defer cn.mtx.Unlock()
	cn.deadline = t
	for _, nc := range []net.Conn{cn.control, cn.data} {
		if nc != nil {
			_ = nc.SetDeadline(t)
		}
	}
}

// replyCode returns the FTP reply code of err, or zero for an error below
// the protocol.
func replyCode(err error) int {
	var terr *textproto.Error
	if errors.As(err, &terr) {
		return terr.Code
	}
	return 0
}

// isReply reports whether err is a reply from the server.
func isReply(err error) bool {
	return replyCode(err) != 0
}

// isClosing reports whether the server is closing the session.
func isClosing(err error) bool {
	return replyCode(err) == ftp.StatusNotAvailable
}

// isNotFound reports whether the server refused a file as unavailable. The
// same reply covers a file the user may not read, which reads as missing: a
// reader then fetches the block elsewhere, and nothing deletes on a miss.
func isNotFound(err error) bool {
	return replyCode(err) == ftp.StatusFileUnavailable
}

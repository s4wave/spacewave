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
	"sync"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// DefaultMaxSessions is the default most sessions open at once from this
// client, spares included: the limit of ten sessions per client host that
// servers such as ProFTPD enforce.
const DefaultMaxSessions = 10

// DefaultConnsPerLogin is the default most sessions open at once on one
// login: the server's limit of ten per user, less one session of headroom.
const DefaultConnsPerLogin = 9

// fullBackoff is the longest wait between dials while the server reports that
// its session limit is reached.
const fullBackoff = 2 * time.Second

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
	// slots holds the pool: each slot is bound to one login and holds a
	// logged in session, or none when it must dial before use.
	slots chan *slot
	// total holds one token for each session open or being dialed, spares
	// included, which bounds the sessions from this client.
	total chan struct{}
	// tokens holds the same per login, which bounds the sessions on a login.
	tokens []chan struct{}
	// sizes holds the sizes of the objects seen by ListObjects and PutObject,
	// which tell a range read whether it reaches the end of the object.
	sizes sync.Map
}

// slot is a pool entry. conn and spare each hold a token of the slot's login.
type slot struct {
	// login indexes the Config logins.
	login int
	// conn is the logged in session, or nil.
	conn *ftp.ServerConn
	// spare delivers the session dialed ahead for a range read that ends
	// its session early, or nil when none is being dialed. It receives nil
	// when the dial failed.
	spare chan *ftp.ServerConn
}

// NewClient builds a Client. It connects lazily, on the first call. It
// panics when conf has no logins.
func NewClient(conf Config) *Client {
	if len(conf.Logins) == 0 {
		panic("block_store_ftps: no logins")
	}

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

	// Pool one slot fewer than the session cap and than each login allows, so
	// a spare has room, and split the slots across the logins evenly.
	limit := conf.ConnsPerLogin
	if limit <= 0 {
		limit = DefaultConnsPerLogin
	}
	maxSessions := conf.MaxSessions
	if maxSessions <= 0 {
		maxSessions = DefaultMaxSessions
	}
	n := min(max(limit-1, 1)*len(conf.Logins), max(maxSessions-1, 1))
	c := &Client{conf: conf, tlsConf: tlsConf, slots: make(chan *slot, n), total: make(chan struct{}, maxSessions)}
	for range conf.Logins {
		c.tokens = append(c.tokens, make(chan struct{}, limit))
	}
	for i := range n {
		c.slots <- &slot{login: i % len(conf.Logins)}
	}
	return c
}

// Conns returns the number of pooled sessions, the concurrency the pool can
// serve.
func (c *Client) Conns() int {
	return cap(c.slots)
}

// Close waits for running calls and logs out every session. A later call
// dials again.
func (c *Client) Close() {
	held := make([]*slot, 0, cap(c.slots))
	for range cap(c.slots) {
		s := <-c.slots
		c.drop(s)
		held = append(held, s)
	}
	for _, s := range held {
		c.slots <- s
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

	err := c.do(ctx, func(conn *ftp.ServerConn, _ *slot) error {
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
	if err == nil {
		c.sizes.Store(name, int64(len(data)))
	}
	return err
}

// GetObject reads the whole object. Packfile readers read whole objects only
// to decode them, so the body is held in memory and the connection returns to
// the pool at once.
func (c *Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	// Download the object whole on one connection.
	name := path.Join(bucket, key)
	var data []byte
	err := c.do(ctx, func(conn *ftp.ServerConn, _ *slot) error {
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
// ends the session on the early close, as ProFTPD does, spends the session,
// so a range that does not reach the end of the object, as far as the sizes
// seen by ListObjects and PutObject say, starts dialing the slot's
// replacement before it reads, and the next call finds it ready.
func (c *Client) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	// Read the range on one connection.
	name := path.Join(bucket, key)
	var out []byte
	err := c.do(ctx, func(conn *ftp.ServerConn, s *slot) error {
		// Dial the replacement while the transfer runs, unless the range
		// reaches the end of the object and ends the transfer by itself.
		if size, ok := c.sizes.Load(name); !ok || off+int64(length) < size.(int64) {
			c.startSpare(ctx, s)
		}

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
	// Remember the sizes for the range reads.
	inner := fn
	fn = func(key string, size int64) error {
		c.sizes.Store(path.Join(bucket, key), size)
		return inner(key, size)
	}
	dir, _ := path.Split(prefix)
	return c.do(ctx, func(conn *ftp.ServerConn, _ *slot) error {
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
	c.sizes.Delete(name)
	return c.do(ctx, func(conn *ftp.ServerConn, _ *slot) error {
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
func (c *Client) do(ctx context.Context, fn func(conn *ftp.ServerConn, s *slot) error) error {
	// Take a slot with a connection from the pool.
	s, err := c.acquire(ctx)
	if err != nil {
		return err
	}
	conn := s.conn

	// Retry on a fresh connection when the pooled one failed. The new
	// connection keeps the slot's token.
	err = fn(conn, s)
	if err != nil && err != errSpent && !isReply(err) {
		_ = conn.Quit()
		conn, err = c.dial(ctx, s.login)
		if err == nil {
			s.conn = conn
			err = fn(conn, s)
		} else {
			s.conn = nil
			c.free(s.login)
		}
	}

	// Return the slot, dropping a failed or spent connection.
	c.release(s, err)
	if err == errSpent {
		return nil
	}
	return err
}

// acquire takes a slot and gives it a connection: the spare dialed for it, or
// a new dial. On error the slot is already back in the pool.
func (c *Client) acquire(ctx context.Context) (*slot, error) {
	// Wait for a free slot.
	var s *slot
	select {
	case s = <-c.slots:
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
	if s.conn != nil {
		return s, nil
	}

	// Wait for the spare dialed while the last session ran.
	if s.spare != nil {
		select {
		case s.conn = <-s.spare:
			s.spare = nil
			if s.conn == nil {
				c.free(s.login)
			}
		case <-ctx.Done():
			c.slots <- s
			return nil, context.Cause(ctx)
		}
		if s.conn != nil {
			return s, nil
		}
	}

	// Dial the empty slot under a token of its login.
	if err := c.take(ctx, s.login); err != nil {
		c.slots <- s
		return nil, err
	}
	conn, err := c.dial(ctx, s.login)
	if err != nil {
		c.free(s.login)
		c.slots <- s
		return nil, err
	}
	s.conn = conn
	return s, nil
}

// release returns a slot to the pool, keeping its connection unless err shows
// the connection failed or spent. A connection that outlived its range read
// makes the spare dialed for it surplus.
func (c *Client) release(s *slot, err error) {
	if s.conn != nil && err != nil && !isReply(err) {
		_ = s.conn.Quit()
		s.conn = nil
		c.free(s.login)
	}
	if s.conn != nil && s.spare != nil {
		go c.discard(s.login, s.spare)
		s.spare = nil
	}
	c.slots <- s
}

// startSpare dials a replacement for the slot's session in the background,
// when its login has a session to spare. The dial is bounded by dialTimeout.
func (c *Client) startSpare(ctx context.Context, s *slot) {
	if s.spare != nil {
		return
	}
	if !c.tryTake(s.login) {
		return
	}
	spare := make(chan *ftp.ServerConn, 1)
	s.spare = spare
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*dialTimeout)
	go func() {
		defer cancel()
		conn, err := c.dial(ctx, s.login)
		if err != nil {
			conn = nil
		}
		spare <- conn
	}()
}

// discard closes the spare session and frees its token.
func (c *Client) discard(login int, spare chan *ftp.ServerConn) {
	if conn := <-spare; conn != nil {
		_ = conn.Quit()
	}
	c.free(login)
}

// drop logs out the sessions of a slot held by the caller.
func (c *Client) drop(s *slot) {
	if s.conn != nil {
		_ = s.conn.Quit()
		s.conn = nil
		c.free(s.login)
	}
	if s.spare != nil {
		c.discard(s.login, s.spare)
		s.spare = nil
	}
}

// take waits for a session token of the login and of the client.
func (c *Client) take(ctx context.Context, login int) error {
	select {
	case c.tokens[login] <- struct{}{}:
	case <-ctx.Done():
		return context.Cause(ctx)
	}
	select {
	case c.total <- struct{}{}:
		return nil
	case <-ctx.Done():
		<-c.tokens[login]
		return context.Cause(ctx)
	}
}

// tryTake takes the tokens of a session without waiting, and reports whether
// it did.
func (c *Client) tryTake(login int) bool {
	select {
	case c.tokens[login] <- struct{}{}:
	default:
		return false
	}
	select {
	case c.total <- struct{}{}:
		return true
	default:
		<-c.tokens[login]
		return false
	}
}

// free returns the tokens of a session of the login.
func (c *Client) free(login int) {
	<-c.total
	<-c.tokens[login]
}

// dial connects, upgrades to TLS with AUTH TLS, and logs in. While the server
// answers that its limit of sessions is reached, which another client of the
// host may cause, it waits with a growing delay and tries again until ctx ends.
func (c *Client) dial(ctx context.Context, login int) (*ftp.ServerConn, error) {
	wait := 100 * time.Millisecond
	for {
		conn, err := c.dialOnce(ctx, login)
		if err == nil || !isFull(err) {
			return conn, err
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, err
		}
		wait = min(2*wait, fullBackoff)
	}
}

// dialOnce makes one attempt to dial.
func (c *Client) dialOnce(ctx context.Context, login int) (*ftp.ServerConn, error) {
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
	if err := conn.Login(c.conf.Logins[login].User, c.conf.Logins[login].Password); err != nil {
		_ = conn.Quit()
		return nil, errors.Wrap(err, "log in to "+c.conf.Addr)
	}
	return conn, nil
}

// isFull reports whether err is the server's answer that it holds the most
// sessions it allows the user or the client host, sent at the greeting or the
// login, as "530 Sorry, the maximum number of clients (10) for this user are
// already connected".
func isFull(err error) bool {
	var reply *textproto.Error
	return errors.As(err, &reply) && (reply.Code == 530 || reply.Code == 421) &&
		strings.Contains(reply.Msg, "maximum number")
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

// Package block_store_sftp stores the objects of a block_store_s3.PackStore on
// an SFTP server.
//
// The client holds a few long-lived SSH connections, each carrying many
// requests at once, and keeps the files it reads open between range reads, so
// a range read costs one round trip and no login. This differs from FTPS,
// where a range read ends its transfer and with it, on some servers, its
// session.
package block_store_sftp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/pkg/sftp"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"golang.org/x/crypto/ssh"
)

// DefaultConns is the default number of SSH connections.
const DefaultConns = 4

// dialTimeout bounds connecting and the SSH handshake.
const dialTimeout = 30 * time.Second

// maxDials bounds the handshakes in progress at once. OpenSSH's MaxStartups
// starts refusing unauthenticated connections at 10 by default, so a client
// with thousands of connections opens them a few at a time.
const maxDials = 8

// dialAttempts bounds the attempts of one dial, and dialBackoff is the wait
// after the first failure, doubled after each later one.
const (
	dialAttempts = 6
	dialBackoff  = 250 * time.Millisecond
)

// keepaliveInterval is the time between keepalive requests on an idle
// connection, and keepaliveTimeout the wait for the answer. A connection that
// does not answer is closed, which fails the requests stuck on it.
const (
	keepaliveInterval = 30 * time.Second
	keepaliveTimeout  = 15 * time.Second
)

// idleTimeout is how long an open file stays unused before it is closed.
const idleTimeout = time.Minute

// maxOpenFiles is the most files kept open on one connection. Past it, the
// least recently used unused file is closed to make room.
const maxOpenFiles = 64

// tempPrefix starts the name of an object being written. ListObjects skips
// such names.
const tempPrefix = ".tmp-"

// Client is a block_store_s3.ObjectStore on an SFTP server.
//
// A bucket is a directory relative to the login directory, and a key is a
// slash path below it; missing directories are created on write. Calls are
// spread across the connections in turn. A call whose connection failed
// closes it and runs once more on a fresh one, and the next call on the slot
// finds the new connection ready.
//
// The SFTP requests do not take a context: ctx ends the wait for the result,
// and a request already sent runs until the server answers or the connection
// is closed by its keepalive.
type Client struct {
	// conf is the configuration with Addr carrying a port.
	conf Config
	// sshConf is the SSH configuration shared by every connection.
	sshConf *ssh.ClientConfig
	// slots holds one entry per SSH connection.
	slots []*slot
	// next counts calls, to pick the slot of the next.
	next atomic.Uint32
	// dials holds a token for each handshake in progress.
	dials chan struct{}
}

// slot holds one SSH connection, dialed when first needed.
type slot struct {
	c *Client
	// mu guards conn and the files of the connection in it.
	mu   sync.Mutex
	conn *conn
}

// conn is one SSH connection with its SFTP session and open files. Its files
// are guarded by the mutex of its slot.
type conn struct {
	slot *slot
	ssh  *ssh.Client
	sftp *sftp.Client
	// files holds the files open on the connection by name.
	files map[string]*openFile
	// timer closes the unused files when they idle; nil when no file is open.
	timer *time.Timer
	// closed is set when the connection is closed.
	closed bool
	// done ends the keepalive.
	done chan struct{}
}

// openFile is a file kept open for range reads.
type openFile struct {
	// ready is closed when the open finished, with f or err set.
	ready chan struct{}
	f     *sftp.File
	err   error
	// refs counts the reads using the file. last is when it was last used.
	refs int
	last time.Time
	// dropped is set when the file left the cache: the last read to finish
	// closes it.
	dropped bool
}

// NewClient builds a Client. It connects lazily, on the first call. It fails
// when conf.HostKey is not a fingerprint or a public key.
func NewClient(conf Config) (*Client, error) {
	// Default the port and the connection count.
	if _, _, err := net.SplitHostPort(conf.Addr); err != nil {
		conf.Addr = net.JoinHostPort(conf.Addr, "22")
	}
	if conf.Conns <= 0 {
		conf.Conns = DefaultConns
	}

	// Verify the server by its host key.
	hostKey, algos, err := newHostKeyCallback(conf.HostKey)
	if err != nil {
		return nil, err
	}

	// Build the client with its empty slots.
	c := &Client{
		dials: make(chan struct{}, maxDials),
		conf:  conf,
		sshConf: &ssh.ClientConfig{
			User:              conf.User,
			Auth:              []ssh.AuthMethod{ssh.Password(conf.Password)},
			HostKeyCallback:   hostKey,
			HostKeyAlgorithms: algos,
			Timeout:           dialTimeout,
		},
	}
	for range conf.Conns {
		c.slots = append(c.slots, &slot{c: c})
	}
	return c, nil
}

// Close closes every connection. A later call dials again.
func (c *Client) Close() {
	for _, s := range c.slots {
		s.mu.Lock()
		cn := s.conn
		s.conn = nil
		s.mu.Unlock()
		if cn != nil {
			cn.close()
		}
	}
}

// PutObject writes the object under a temporary name in its directory and
// renames it into place, replacing any object, so a reader never sees it
// partly written.
func (c *Client) PutObject(ctx context.Context, bucket, key string, data []byte, _ string) error {
	// Name the temporary file beside the object.
	name := path.Join(bucket, key)
	dir, base := path.Split(name)
	tag := make([]byte, 8)
	_, _ = rand.Read(tag)
	tmp := dir + tempPrefix + hex.EncodeToString(tag) + "-" + base

	// Write the object on one connection.
	err := c.do(ctx, func(cn *conn) error {
		// Upload the temporary file, creating the directories it needs.
		err := write(cn.sftp, tmp, data)
		if isNotExist(err) && dir != "" {
			if err = cn.sftp.MkdirAll(strings.TrimSuffix(dir, "/")); err == nil {
				err = write(cn.sftp, tmp, data)
			}
		}
		if err != nil {
			return errors.Wrap(err, "store "+name)
		}

		// Rename it into place over any object there.
		err = cn.sftp.PosixRename(tmp, name)
		if err != nil && isReply(err) {
			// The server lacks the extension: replace the object with a
			// delete and a rename, which leaves a moment without it.
			_ = cn.sftp.Remove(name)
			err = cn.sftp.Rename(tmp, name)
		}
		if err != nil {
			_ = cn.sftp.Remove(tmp)
			return errors.Wrap(err, "rename "+name)
		}
		return nil
	})
	c.forget(name)
	return err
}

// write creates the file and writes data to it.
func write(sc *sftp.Client, name string, data []byte) error {
	// Create the file and write all of data.
	f, err := sc.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// GetObject reads the whole object, which the caller decodes in memory.
func (c *Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	// Read the object whole on one connection.
	name := path.Join(bucket, key)
	var buf bytes.Buffer
	err := c.do(ctx, func(cn *conn) error {
		// Open the object and copy it, with concurrent reads.
		buf.Reset()
		f, err := cn.sftp.Open(name)
		if err != nil {
			return notFound(err, name)
		}
		_, err = f.WriteTo(&buf)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return io.NopCloser(&buf), nil
}

// GetObjectRange reads length bytes from offset off, exactly that range and
// no more, on a file kept open for the next range read. The result is shorter
// than length when the object ends first.
func (c *Client) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	// Read the range on one connection.
	name := path.Join(bucket, key)
	var out []byte
	err := c.do(ctx, func(cn *conn) error {
		// Take the open file, and read exactly length bytes from it.
		of, err := cn.acquire(name)
		if err != nil {
			return notFound(err, name)
		}
		buf := make([]byte, length)
		n, err := of.f.ReadAt(buf, off)
		if err == io.EOF {
			err = nil
		}
		cn.release(name, of, err)
		out = buf[:n]
		return err
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
	return c.do(ctx, func(cn *conn) error {
		return walk(cn.sftp, bucket, dir, prefix, fn)
	})
}

// walk lists the directory dir of bucket, calling fn for the files whose keys
// start with prefix and descending into the directories that may hold some.
func walk(sc *sftp.Client, bucket, dir, prefix string, fn func(key string, size int64) error) error {
	// List the directory, in name order.
	entries, err := sc.ReadDir(path.Join(bucket, dir))
	if isNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	slices.SortFunc(entries, func(a, b os.FileInfo) int { return strings.Compare(a.Name(), b.Name()) })

	// Visit each file and descend into each directory under the prefix.
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, tempPrefix) {
			continue
		}
		key := dir + name
		switch {
		case entry.Mode().IsRegular():
			if strings.HasPrefix(key, prefix) {
				if err := fn(key, entry.Size()); err != nil {
					return err
				}
			}
		case entry.IsDir():
			sub := key + "/"
			if strings.HasPrefix(sub, prefix) || strings.HasPrefix(prefix, sub) {
				if err := walk(sc, bucket, sub, prefix, fn); err != nil {
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
	err := c.do(ctx, func(cn *conn) error {
		if err := cn.sftp.Remove(name); err != nil && !isNotExist(err) {
			return err
		}
		return nil
	})
	c.forget(name)
	return err
}

// forget drops the open files of the object on every connection, since the
// name no longer holds what they opened.
func (c *Client) forget(name string) {
	for _, s := range c.slots {
		s.mu.Lock()
		var closing *sftp.File
		if cn := s.conn; cn != nil {
			closing = cn.drop(name)
		}
		s.mu.Unlock()
		if closing != nil {
			_ = closing.Close()
		}
	}
}

// do runs fn on the connection of the next slot, dialing it when the slot has
// none. When fn fails on the connection itself, rather than with a server
// reply, the connection is closed and fn runs once more on a fresh one, since
// an idle connection the server closed fails only on first use.
func (c *Client) do(ctx context.Context, fn func(cn *conn) error) error {
	s := c.slots[c.next.Add(1)%uint32(len(c.slots))] //nolint:gosec // the slot count is small.
	var err error
	for range 2 {
		var cn *conn
		if cn, err = s.connect(ctx); err != nil {
			return err
		}
		if err = call(ctx, func() error { return fn(cn) }); err == nil || isReply(err) || ctx.Err() != nil {
			return err
		}
		s.reset(cn)
	}
	return err
}

// call runs fn and returns its result, or the context's cause when ctx ends
// first. The request fn made keeps running in that case.
func call(ctx context.Context, fn func() error) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// connect returns the slot's connection, dialing it first when it has none.
// The slot is held for the dial, so the calls on a dropped slot wait for its
// new connection together.
func (s *slot) connect(ctx context.Context) (*conn, error) {
	// Reuse the slot's connection.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		return s.conn, nil
	}

	// Dial a new one.
	cn, err := s.c.dial(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		return nil, &ConnectError{Err: err}
	}
	cn.slot = s
	s.conn = cn
	return cn, nil
}

// reset closes cn, the failed connection of the slot, unless the slot already
// replaced it.
func (s *slot) reset(cn *conn) {
	s.mu.Lock()
	if s.conn == cn {
		s.conn = nil
	}
	s.mu.Unlock()
	cn.close()
}

// dial connects, runs the SSH handshake and logs in, then starts the SFTP
// session and the keepalive. A server under load drops handshakes and refuses
// sessions past its limits, so a failed attempt is retried with backoff.
func (c *Client) dial(ctx context.Context) (*conn, error) {
	// Wait for a turn to dial.
	select {
	case c.dials <- struct{}{}:
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
	defer func() { <-c.dials }()

	// Try until an attempt succeeds, the attempts run out or ctx ends.
	wait := dialBackoff
	for attempt := 1; ; attempt++ {
		cn, err := c.dialOnce(ctx)
		if err == nil || attempt == dialAttempts {
			return cn, err
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		}
		wait *= 2
	}
}

// dialOnce makes one attempt to connect, log in and start the SFTP session.
func (c *Client) dialOnce(ctx context.Context) (*conn, error) {
	// Connect and run the handshake. The error names the server, never the
	// credentials.
	d := net.Dialer{Timeout: dialTimeout}
	nc, err := d.DialContext(ctx, "tcp", c.conf.Addr)
	if err != nil {
		return nil, errors.Wrap(err, "connect to "+c.conf.Addr)
	}
	_ = nc.SetDeadline(time.Now().Add(dialTimeout))
	sc, chans, reqs, err := ssh.NewClientConn(nc, c.conf.Addr, c.sshConf)
	if err != nil {
		_ = nc.Close()
		return nil, errors.Wrap(err, "log in to "+c.conf.Addr)
	}
	_ = nc.SetDeadline(time.Time{})
	client := ssh.NewClient(sc, chans, reqs)

	// Start the SFTP session.
	sftpc, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		return nil, errors.Wrap(err, "start sftp on "+c.conf.Addr)
	}
	cn := &conn{ssh: client, sftp: sftpc, files: map[string]*openFile{}, done: make(chan struct{})}
	go cn.keepalive()
	return cn, nil
}

// keepalive closes the connection when the server stops answering, which
// fails the requests waiting on it and lets the next call dial again.
func (cn *conn) keepalive() {
	t := time.NewTicker(keepaliveInterval)
	defer t.Stop()
	for {
		select {
		case <-cn.done:
			return
		case <-t.C:
		}
		answered := make(chan struct{})
		go func() {
			_, _, _ = cn.ssh.SendRequest("keepalive@openssh.com", true, nil)
			close(answered)
		}()
		select {
		case <-answered:
		case <-time.After(keepaliveTimeout):
			_ = cn.ssh.Close()
			return
		case <-cn.done:
			return
		}
	}
}

// close ends the SFTP session and the connection, which closes its files.
func (cn *conn) close() {
	// Mark the connection closed once.
	cn.slot.mu.Lock()
	if cn.closed {
		cn.slot.mu.Unlock()
		return
	}
	cn.closed = true
	if cn.timer != nil {
		cn.timer.Stop()
	}
	cn.slot.mu.Unlock()

	// End the keepalive and the sessions.
	close(cn.done)
	_ = cn.sftp.Close()
	_ = cn.ssh.Close()
}

// acquire returns the open file of name for a read, opening it when the
// connection has none. Concurrent reads of one name share the open. The
// caller hands it back with release.
func (cn *conn) acquire(name string) (*openFile, error) {
	// Share the file another read opened or is opening.
	s := cn.slot
	s.mu.Lock()
	of := cn.files[name]
	if of != nil {
		of.refs++
		of.last = time.Now()
		s.mu.Unlock()
		<-of.ready
		if of.err != nil {
			// The opener already dropped the entry.
			return nil, of.err
		}
		return of, nil
	}

	// Register the file and make room for it.
	of = &openFile{ready: make(chan struct{}), refs: 1, last: time.Now()}
	closing := cn.evict()
	cn.files[name] = of
	cn.arm()
	s.mu.Unlock()

	// Close the evicted files, then open this one without holding the slot.
	for _, f := range closing {
		_ = f.Close()
	}
	of.f, of.err = cn.sftp.Open(name)

	// Publish the result to the reads waiting on the open.
	if of.err != nil {
		s.mu.Lock()
		cn.unlink(name, of)
		s.mu.Unlock()
	}
	close(of.ready)
	if of.err != nil {
		return nil, of.err
	}
	return of, nil
}

// release ends a read of the file. A file whose read failed with anything but
// a reply of the server is dropped, and closed when the last read ends.
func (cn *conn) release(name string, of *openFile, err error) {
	// Uncount the read, dropping the file after a failure.
	s := cn.slot
	s.mu.Lock()
	if err != nil {
		cn.unlink(name, of)
	}
	of.refs--
	of.last = time.Now()
	var closing *sftp.File
	if of.dropped && of.refs == 0 {
		closing = of.f
	}
	s.mu.Unlock()

	// Close a dropped file the last read left.
	if closing != nil {
		_ = closing.Close()
	}
}

// unlink removes the file from the cache and marks it dropped. The caller
// holds the slot.
func (cn *conn) unlink(name string, of *openFile) {
	if cn.files[name] == of {
		delete(cn.files, name)
	}
	of.dropped = true
}

// drop removes the named file from the cache and returns it to close when no
// read uses it. The last read closes a file in use. The caller holds the slot.
func (cn *conn) drop(name string) *sftp.File {
	// Unlink the file, and hand it back when no read holds it.
	of := cn.files[name]
	if of == nil {
		return nil
	}
	cn.unlink(name, of)
	if of.refs == 0 && of.f != nil {
		return of.f
	}
	return nil
}

// evict makes room for one more file by dropping the least recently used
// unused files, and returns them to close once the slot is released. The
// caller holds the slot.
func (cn *conn) evict() []*sftp.File {
	var closing []*sftp.File
	for len(cn.files) >= maxOpenFiles {
		var oldest string
		var oldestFile *openFile
		for name, of := range cn.files {
			if of.refs == 0 && (oldestFile == nil || of.last.Before(oldestFile.last)) {
				oldest, oldestFile = name, of
			}
		}
		if oldestFile == nil {
			break
		}
		closing = append(closing, cn.drop(oldest))
	}
	return closing
}

// arm starts the timer that closes idle files, when it is not running. The
// caller holds the slot.
func (cn *conn) arm() {
	if cn.timer == nil && !cn.closed {
		cn.timer = time.AfterFunc(idleTimeout, cn.sweep)
	}
}

// sweep closes the files unused for the idle timeout and sets the timer again
// while files remain.
func (cn *conn) sweep() {
	// Drop the idle files and set the timer again while files remain.
	s := cn.slot
	s.mu.Lock()
	var closing []*sftp.File
	cutoff := time.Now().Add(-idleTimeout)
	for name, of := range cn.files {
		if of.refs == 0 && of.last.Before(cutoff) {
			closing = append(closing, cn.drop(name))
		}
	}
	cn.timer = nil
	if len(cn.files) != 0 {
		cn.arm()
	}
	s.mu.Unlock()

	// Close them outside the slot.
	for _, f := range closing {
		_ = f.Close()
	}
}

// isReply reports whether err is an answer of the server, which leaves the
// connection usable, rather than a failure of the connection.
func isReply(err error) bool {
	var status *sftp.StatusError
	return err == nil || err == io.EOF || errors.As(err, &status) || isNotExist(err) || errors.Is(err, block_store_s3.ErrNotFound) || errors.Is(err, os.ErrPermission)
}

// isNotExist reports whether err is the server's answer that a path is
// missing.
func isNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}

// notFound marks a missing object as block_store_s3.ErrNotFound.
func notFound(err error, name string) error {
	if isNotExist(err) {
		return errors.Wrap(block_store_s3.ErrNotFound, name)
	}
	return err
}

// _ is a type assertion
var _ block_store_s3.ObjectStore = (*Client)(nil)

package provider_local

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"time"

	block_store_sftp "github.com/s4wave/spacewave/db/block/store/sftp"
	"github.com/sirupsen/logrus"
)

// sftpHold is how long calls skip SFTP after it failed to connect, before the
// next call tries it again.
const sftpHold = 5 * time.Second

// muxServer reaches one directory over SFTP and FTPS at once. Each call goes
// to the protocol with fewer calls in flight, SFTP on a tie, so the faster
// protocol carries more of them. A call SFTP cannot open a connection for
// runs over FTPS, and calls skip SFTP for sftpHold after that.
type muxServer struct {
	le   *logrus.Entry
	sftp packServer
	ftps packServer
	// sftpCalls and ftpsCalls count the calls in flight on each protocol.
	sftpCalls atomic.Int64
	ftpsCalls atomic.Int64
	// held is the UnixNano time before which calls skip SFTP, zero while SFTP
	// connects.
	held atomic.Int64
}

// newMuxServer builds a muxServer of an SFTP and an FTPS client of the same
// directory.
func newMuxServer(le *logrus.Entry, sftp, ftps packServer) *muxServer {
	return &muxServer{le: le, sftp: sftp, ftps: ftps}
}

// muxCall runs fn on the protocol m picks, and on FTPS when SFTP cannot
// connect.
func muxCall[T any](ctx context.Context, m *muxServer, fn func(packServer) (T, error)) (T, error) {
	// Pick SFTP unless it is held or has more calls in flight.
	held := m.held.Load()
	if (held == 0 || time.Now().UnixNano() >= held) && m.sftpCalls.Load() <= m.ftpsCalls.Load() {
		m.sftpCalls.Add(1)
		v, err := fn(m.sftp)
		m.sftpCalls.Add(-1)
		var connectErr *block_store_sftp.ConnectError
		if !errors.As(err, &connectErr) || ctx.Err() != nil {
			if err == nil && held != 0 && m.held.CompareAndSwap(held, 0) {
				m.le.Info("the SFTP server accepts connections again")
			}
			return v, err
		}
		if m.held.Swap(time.Now().Add(sftpHold).UnixNano()) == 0 {
			m.le.WithError(err).Warn("the SFTP server refused a connection; calls go over FTPS")
		}
	}

	// Run the call over FTPS.
	m.ftpsCalls.Add(1)
	defer m.ftpsCalls.Add(-1)
	return fn(m.ftps)
}

// PutObject writes the object.
func (m *muxServer) PutObject(ctx context.Context, bucket, key string, data []byte, contentType string) error {
	_, err := muxCall(ctx, m, func(s packServer) (struct{}, error) {
		return struct{}{}, s.PutObject(ctx, bucket, key, data, contentType)
	})
	return err
}

// GetObject opens the object's body.
func (m *muxServer) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	return muxCall(ctx, m, func(s packServer) (io.ReadCloser, error) {
		return s.GetObject(ctx, bucket, key)
	})
}

// GetObjectRange reads length bytes from offset off.
func (m *muxServer) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	return muxCall(ctx, m, func(s packServer) ([]byte, error) {
		return s.GetObjectRange(ctx, bucket, key, off, length)
	})
}

// ListObjects lists the objects under prefix. SFTP fails to connect before it
// calls fn, so FTPS never repeats a listed key.
func (m *muxServer) ListObjects(ctx context.Context, bucket, prefix string, fn func(key string, size int64) error) error {
	_, err := muxCall(ctx, m, func(s packServer) (struct{}, error) {
		return struct{}{}, s.ListObjects(ctx, bucket, prefix, fn)
	})
	return err
}

// DeleteObject removes the object.
func (m *muxServer) DeleteObject(ctx context.Context, bucket, key string) error {
	_, err := muxCall(ctx, m, func(s packServer) (struct{}, error) {
		return struct{}{}, s.DeleteObject(ctx, bucket, key)
	})
	return err
}

// Close closes the connections of both protocols.
func (m *muxServer) Close() {
	m.sftp.Close()
	m.ftps.Close()
}

// _ is a type assertion
var _ packServer = (*muxServer)(nil)

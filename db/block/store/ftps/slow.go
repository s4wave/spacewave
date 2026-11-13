package block_store_ftps

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"net/textproto"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/errors"
)

// slowCloseWait is the default bound on the wait for the replies that show a
// session in step after a slow transfer's close.
const slowCloseWait = 2 * time.Second

// ErrSlow matches every SlowError: a transfer GetObjectMaxSlow abandoned
// for its rate. It is not retried on a fresh connection.
var ErrSlow = errors.New("ftps transfer under the slow rate")

// ErrSlowEnded matches a SlowError whose close ended the session: the server
// closed it, or its replies were not shown in step within SlowCloseWait.
var ErrSlowEnded = errors.New("ftps session ended at a slow transfer's close")

// SlowCheck asks GetObjectMaxSlow to abandon a transfer whose rate is under
// Rate bytes per second once Grace has passed. A zero Rate or Grace is no
// check.
type SlowCheck struct {
	// Rate is the least bytes per second a transfer may run at past Grace.
	Rate int64
	// Grace is the time a transfer runs before Rate is checked, once.
	Grace time.Duration
}

// on reports whether the check is asked for.
func (k SlowCheck) on() bool {
	return k.Rate > 0 && k.Grace > 0
}

// SlowError reports a transfer abandoned for its rate: the bytes it read in
// the time it took. Ended is set when the close ended the session, with
// Cause telling why; otherwise the session is kept, in step, for the next
// call. It wraps no error, so it never reads as a server reply.
type SlowError struct {
	// Bytes is the count of bytes the transfer read before its close.
	Bytes int64
	// Elapsed is the time from the transfer's start to the rate check.
	Elapsed time.Duration
	// Ended is set when the close ended the session.
	Ended bool
	// Cause says why the session ended, with the reply codes read, and no
	// server text; empty when it was kept.
	Cause string
	// Close is the reply shape read for the close of a kept session: "426",
	// "226" or "426+226", each followed by the NOOP's 200.
	Close string
}

// Rate returns the transfer's rate in bytes per second.
func (e *SlowError) Rate() int64 {
	ms := max(e.Elapsed.Milliseconds(), 1)
	return e.Bytes * 1000 / ms
}

// Error describes the abandoned transfer with numbers and, when the session
// ended, the cause, which holds no server text.
func (e *SlowError) Error() string {
	s := fmt.Sprintf("ftps slow transfer closed after %d bytes in %d ms", e.Bytes, e.Elapsed.Milliseconds())
	if e.Ended {
		s += ", session ended (" + e.Cause + ")"
	}
	return s
}

// Is matches ErrSlow, and ErrSlowEnded when the session ended.
func (e *SlowError) Is(target error) bool {
	return target == ErrSlow || (e.Ended && target == ErrSlowEnded)
}

// slowKept reports whether err is a slow transfer whose session is kept.
func slowKept(err error) bool {
	return errors.Is(err, ErrSlow) && !errors.Is(err, ErrSlowEnded)
}

// GetObjectMaxSlow reads the whole object as GetObjectMax does, abandoning
// a transfer still under slow.Rate once slow.Grace has passed: it closes the
// data connection and returns a *SlowError. The session is kept only when
// the server's reply to the close was read and a NOOP's 200 followed it, so
// replies and commands are in step; otherwise it is ended. A 421 or 530 in
// those replies is returned as the server's reply. With no check asked for,
// it is GetObjectMax.
func (c *Client) GetObjectMaxSlow(ctx context.Context, bucket, key string, limit int64, slow SlowCheck) ([]byte, error) {
	// Read as GetObjectMax does with no check.
	if !slow.on() {
		return c.GetObjectMax(ctx, bucket, key, limit)
	}

	// Download on a pooled connection, abandoning a slow transfer.
	name := path.Join(bucket, key)
	var data []byte
	err := c.do(ctx, func(conn *ftp.ServerConn, _ *slot) error {
		// Start the download.
		resp, err := conn.Retr(name)
		if err != nil {
			return notFound(err, name)
		}

		// Read up to one byte past the limit, checking the rate once after
		// the grace.
		var got int64
		data, got, err = readSlow(resp, limit+1, slow)
		if se, ok := err.(*SlowError); ok {
			data = nil
			return c.slowClose(conn, se, got)
		}

		// Fail an object past the limit, as GetObjectMax does.
		if err == nil && int64(len(data)) > limit {
			data = nil
			if cerr := resp.Close(); endsSession(cerr) {
				return cerr
			}
			return errors.Wrapf(ErrTooLarge, "%s is over %d bytes", name, limit)
		}
		return closeTransfer(resp, err)
	})
	return data, err
}

// readSlow reads r to its end or n bytes. At the first read that returns
// after slow.Grace, a rate under slow.Rate ends the read with a *SlowError
// holding the time taken; the check runs once.
func readSlow(r io.Reader, n int64, slow SlowCheck) ([]byte, int64, error) {
	// Read in chunks, checking the rate at the first return past the grace.
	var buf bytes.Buffer
	lr := io.LimitReader(r, n)
	chunk := make([]byte, 64<<10)
	start := time.Now()
	checked := false
	for {
		k, err := lr.Read(chunk)
		buf.Write(chunk[:k])
		if err == io.EOF {
			return buf.Bytes(), int64(buf.Len()), nil
		}
		if err != nil {
			return buf.Bytes(), int64(buf.Len()), err
		}
		if el := time.Since(start); !checked && el >= slow.Grace {
			checked = true
			got := int64(buf.Len())
			if got*int64(time.Second) < slow.Rate*int64(el) {
				return nil, got, &SlowError{Bytes: got, Elapsed: el}
			}
		}
	}
}

// slowClose ends an abandoned transfer: it closes the data connection, sends
// NOOP at once, and reads the replies under SlowCloseWait. The session is in
// step on 426, 226 or 426 then 226, then 200: se is returned with the
// session kept. A 421 or 530 is returned as the reply; any other reply, a
// 200 first, a read error or the wait's end ends the session.
func (c *Client) slowClose(conn *ftp.ServerConn, se *SlowError, got int64) error {
	// Find the session's control connection and the library's reader.
	se.Bytes = got
	end := func(cause string) error {
		se.Ended, se.Cause = true, cause
		return se
	}
	s := c.sess.lookup(conn)
	tc := textConn(conn)
	if s == nil || tc == nil {
		return end("no tracked control connection")
	}
	ctl := s.closeData()
	if ctl == nil {
		return end("session closed")
	}

	// Bound the replies' wait; the kept path clears the deadline below, and
	// the session's idle after the call clears it too.
	wait := cmp.Or(c.conf.SlowCloseWait, slowCloseWait)
	if err := ctl.SetDeadline(time.Now().Add(wait)); err != nil {
		return end("deadline")
	}
	if _, err := tc.Cmd("NOOP"); err != nil {
		return end("noop not sent")
	}

	// Read the close's replies, then the NOOP's 200, naming the codes read
	// in the cause of an ended session.
	var saw426, saw226 bool
	var codes []string
	ended := func(cause string) error {
		if len(codes) != 0 {
			cause += ", replies " + strings.Join(codes, "+")
		}
		return end(cause)
	}
	for {
		code, msg, err := tc.ReadResponse(0)
		if err != nil {
			var reply *textproto.Error
			if errors.As(err, &reply) {
				return ended(fmt.Sprintf("reply %d unreadable", reply.Code))
			}
			return ended("no reply in time or the connection closed")
		}
		codes = append(codes, strconv.Itoa(code))
		switch {
		case code == 421 || code == 530:
			return &textproto.Error{Code: code, Msg: msg}
		case code == 426 && !saw426 && !saw226:
			saw426 = true
		case code == 226 && !saw226:
			saw226 = true
		case code == 200 && (saw426 || saw226):
			// In step: clear the bound, as the session's idle also does.
			if err := ctl.SetDeadline(time.Time{}); err != nil {
				return ended("deadline not cleared")
			}
			se.Close = strings.Join(codes[:len(codes)-1], "+")
			return se
		default:
			return ended(fmt.Sprintf("reply %d out of step", code))
		}
	}
}

// textConn returns the library's control reader of conn, an unexported
// field of ftp.ServerConn (github.com/jlaffaye/ftp v0.2.4), or nil when the
// field is not there as expected.
func textConn(conn *ftp.ServerConn) *textproto.Conn {
	v := reflect.ValueOf(conn).Elem().FieldByName("conn")
	if !v.IsValid() || v.Type() != reflect.TypeFor[*textproto.Conn]() {
		return nil
	}
	return *(**textproto.Conn)(unsafe.Pointer(v.UnsafeAddr()))
}

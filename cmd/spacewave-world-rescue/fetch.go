//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/textproto"
	"path"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	block_store_ftps "github.com/s4wave/spacewave/db/block/store/ftps"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"golang.org/x/time/rate"
)

// objectReader is the part of the FTPS client the rescue reads with.
// *block_store_ftps.Client implements it.
type objectReader interface {
	// GetObject opens the whole object.
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	// GetObjectRange reads length bytes from offset off.
	GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error)
}

// objectSizer asks the server for an object's size. *block_store_ftps.Client
// implements it with SIZE.
type objectSizer interface {
	// ObjectSize returns the object's size in bytes.
	ObjectSize(ctx context.Context, bucket, key string) (int64, error)
}

// objectMaxReader reads a whole object of at most a limit, failing one past
// it without reading it. *block_store_ftps.Client implements it.
type objectMaxReader interface {
	// GetObjectMax reads the whole object, failing one past limit bytes.
	GetObjectMax(ctx context.Context, bucket, key string, limit int64) ([]byte, error)
}

// errOverSize reports an object the server sent past the size the rescue
// expected and charged for. At most one byte past the size is read.
var errOverSize = errors.New("object larger than its size")

// sessionCloser closes every session of the client at once, cutting the
// calls running on them. *block_store_ftps.Client implements it.
type sessionCloser interface {
	// CloseAll closes every session and refuses later calls.
	CloseAll()
}

// spareWatcher reports the failed dials no call returns.
// *block_store_ftps.Client implements it.
type spareWatcher interface {
	// OnSpareError sets the receiver of a failed spare dial's error.
	OnSpareError(fn func(error))
}

// redialWatcher runs a hook before each redial a request makes after its
// pooled connection failed. *block_store_ftps.Client implements it.
type redialWatcher interface {
	// OnRedial sets the hook; an error from it refuses the redial.
	OnRedial(fn func(ctx context.Context, err error) error)
}

// redialLine is the line logged before each redial, and refusedRedialLine
// the one logged instead under StopOnDrop. The rescue watchdog stops the run
// on either, as both match its redial pattern.
const (
	redialLine        = "ftps redial after a connection error: %v\n"
	refusedRedialLine = "ftps redial refused after a connection error: %v\n"
)

// errNoSizer reports that the client cannot ask for an object's size.
var errNoSizer = errors.New("the client cannot ask for an object's size")

// errCapReached reports that the next request would cross the byte cap.
var errCapReached = errors.New("byte cap reached")

// errHardStopped refuses a request after a hard stop.
var errHardStopped = errors.New("not requested: the run hit a hard stop")

// errCut reports a request that was in flight at a hard stop: its session
// was closed under it and what it read is dropped.
var errCut = errors.Wrap(errHardStopped, "cut at the hard stop")

// errTimeout reports that a request outlived its timeout.
var errTimeout = errors.New("request timed out")

// errRedialRefused refuses the redial of a failed connection under
// StopOnDrop.
var errRedialRefused = errors.New("redial refused: no redials in this run")

// dropLine is the line logged when a lost connection stops the run under
// StopOnDrop; run-s1.sh reads it as the drop exit class.
const dropLine = "stop: connection dropped: %v\n"

// slowEndLine is the line logged when a slow transfer's close ended the
// session under StopOnDrop, a stop told apart from a drop: it matches no
// drop pattern, and the run exits exitSlowStop.
const slowEndLine = "stop: slow close ended the session: %v\n"

// reaskLine is the line logged before each re-ask of a slow pack: the
// re-ask's number and the cap, the pack id, then the abandoned transfer's
// bytes, milliseconds and bytes per second, and the reply shape of its
// close: 426, 226 or 426+226, each followed by the NOOP's 200.
const reaskLine = "slow re-ask %d/%d pack %s bytes %d ms %d rate %d close %s\n"

// reaskDoneLine is the line logged when a pack re-asked at least once is
// read whole: the pack id, the number of the ask that read it, counting the
// first ask as 1, and that ask's own bytes, milliseconds and bytes per
// second.
const reaskDoneLine = "slow re-ask done pack %s ask %d bytes %d ms %d rate %d\n"

// objectSlowReader reads a whole object as objectMaxReader does,
// abandoning a slow transfer. *block_store_ftps.Client implements it.
type objectSlowReader interface {
	// GetObjectMaxSlow reads the whole object, failing one past limit bytes
	// and abandoning one under slow's rate after its grace.
	GetObjectMaxSlow(ctx context.Context, bucket, key string, limit int64, slow block_store_ftps.SlowCheck) ([]byte, error)
}

// isDrop reports whether a failed request lost its connection rather than
// got a server answer: a drop, reset, EOF or timeout, a refused redial or a
// spent dial. A missing object, an object past its size and a cancelled
// run are not drops; neither is any FTP reply, which isHardStop judges.
func isDrop(err error) bool {
	// Rule out no error, a cancel and a refusal after a stop.
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, errHardStopped) {
		return false
	}

	// A refused redial or a spent dial follows a lost connection.
	if errors.Is(err, errRedialRefused) || errors.Is(err, block_store_ftps.ErrDialsSpent) {
		return true
	}

	// Any other error but a missing or over-size object and an FTP reply.
	if errors.Is(err, block_store_s3.ErrNotFound) || errors.Is(err, errOverSize) || errors.Is(err, block_store_ftps.ErrTooLarge) {
		return false
	}
	var reply *textproto.Error
	return !errors.As(err, &reply)
}

// hardStopError wraps a server answer that stops the run with no retry.
type hardStopError struct{ err error }

func (e *hardStopError) Error() string { return "hard stop: " + e.err.Error() }
func (e *hardStopError) Unwrap() error { return e.err }

// replyCode matches an FTP reply code of 530 or 421 at the start of an error
// message or after a wrap.
var replyCode = regexp.MustCompile(`(^|: )(530|421)[ -]`)

// limitPhrases are the server messages that say a session or login limit is
// reached.
var limitPhrases = []string{
	"too many connections",
	"too many users",
	"too many sessions",
	"too many clients",
	"maximum number",
	"login limit",
	"already logged in",
	"connection limit",
	"session limit",
}

// isHardStop reports whether err is an FTP reply 530 or 421, or a server
// message that a connection or login limit is reached. Such an answer stops
// the run with no retry: retrying against a server that refuses logins risks
// a ban.
func isHardStop(err error) bool {
	// Match a reply 530 or 421 the client kept as a textproto error.
	if err == nil {
		return false
	}
	var reply *textproto.Error
	if errors.As(err, &reply) && (reply.Code == 530 || reply.Code == 421) {
		return true
	}

	// Match any refused login, whose reply code the FTP library may drop,
	// and the loss of every session token to unconfirmed closes, after which
	// no session can be dialed within the server's limit.
	if errors.Is(err, block_store_ftps.ErrLoginRefused) || errors.Is(err, block_store_ftps.ErrSessionsLost) || errors.Is(err, block_store_ftps.ErrDialsSpent) {
		return true
	}

	// Match the codes and the limit phrases in the error's text.
	msg := strings.ToLower(err.Error())
	if replyCode.MatchString(msg) {
		return true
	}
	for _, p := range limitPhrases {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// isRetryable reports whether a failed request that is not a hard stop may
// be tried again: a timeout, a dropped connection or a transient 4xx reply,
// but not a missing object or a permanent 5xx reply.
func isRetryable(err error) bool {
	if errors.Is(err, block_store_s3.ErrNotFound) || errors.Is(err, context.Canceled) || errors.Is(err, errOverSize) {
		return false
	}
	var reply *textproto.Error
	if errors.As(err, &reply) {
		return reply.Code < 500
	}
	return true
}

// fetchOptions bound the requests of a fetcher.
type fetchOptions struct {
	// Timeout bounds one request attempt.
	Timeout time.Duration
	// Retries is the most attempts after the first.
	Retries int
	// Backoff is the wait before the first retry, doubling up to MaxBackoff.
	Backoff, MaxBackoff time.Duration
	// RequestsPerSec and BytesPerSec cap the request and byte rates across
	// all sessions; zero is no cap.
	RequestsPerSec, BytesPerSec float64
	// CapBytes is the most bytes fetched across runs, counting each attempt
	// at its requested length.
	CapBytes uint64
	// Reserved, when set, makes the byte total durable before each request
	// that reads bytes, so a crash mid-request loses no charged bytes.
	Reserved func(total uint64) error
	// Log, when set, receives one line per redial of a failed connection
	// and the line of a stop on a drop.
	Log io.Writer
	// StopOnDrop makes a lost connection stop the run: a call refuses to
	// redial, and a request that fails without a server answer is a hard
	// stop logged as dropLine.
	StopOnDrop bool
	// SlowRate is the least bytes per second a whole pack's transfer may
	// run at past SlowGrace; a slower one is abandoned and the pack asked
	// for again on the same session. Zero, or a zero SlowGrace or
	// SlowReasks, is no check.
	SlowRate int64
	// SlowGrace is the time a transfer runs before SlowRate is checked,
	// once.
	SlowGrace time.Duration
	// SlowReasks is the most re-asks of one pack; the last ask reads to
	// the end at any rate, so no pack fails for being slow.
	SlowReasks int
}

// slowOn reports whether the slow check is asked for.
func (o fetchOptions) slowOn() bool {
	return o.SlowRate > 0 && o.SlowGrace > 0 && o.SlowReasks > 0
}

// reqStats times the requests made under a context that carries it, from
// the start of the first to the end of the last, and counts them. A nil
// reqStats records nothing.
type reqStats struct {
	mtx         sync.Mutex
	first, last time.Time
	n           int
}

// reqStatsKey is the context key of a reqStats.
type reqStatsKey struct{}

// withReqStats returns ctx carrying st.
func withReqStats(ctx context.Context, st *reqStats) context.Context {
	return context.WithValue(ctx, reqStatsKey{}, st)
}

// begin records the start of a request.
func (s *reqStats) begin() {
	// Count the request, timing the first.
	if s == nil {
		return
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.n == 0 {
		s.first = time.Now()
	}
	s.n++
}

// end records the end of a request.
func (s *reqStats) end() {
	if s == nil {
		return
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.last = time.Now()
}

// result returns the time from the first request's start to the last's end,
// and the request count.
func (s *reqStats) result() (time.Duration, int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.n == 0 {
		return 0, 0
	}
	return s.last.Sub(s.first), s.n
}

// fetcher makes the rescue's requests within the timeout, retry, rate and
// byte limits.
type fetcher struct {
	client objectReader
	bucket string
	opts   fetchOptions
	reqs   *rate.Limiter
	bytes  *rate.Limiter
	// fetched counts the bytes requested across runs, the ledger's total
	// included.
	fetched atomic.Uint64
	// requests counts the request attempts of this run.
	requests atomic.Uint64
	// stopped is set by the first hard stop: no request starts after it.
	stopped atomic.Bool
	// stopOnce runs the first hard stop, and stopErr is its error.
	stopOnce sync.Once
	stopErr  *hardStopError
}

// newFetcher builds a fetcher whose byte count starts at fetched.
func newFetcher(client objectReader, bucket string, opts fetchOptions, fetched uint64) *fetcher {
	// Start the byte count at the ledger's total.
	f := &fetcher{client: client, bucket: bucket, opts: opts}
	f.fetched.Store(fetched)

	// Stop on a refused spare dial as on a refused request.
	if w, ok := client.(spareWatcher); ok {
		w.OnSpareError(func(err error) {
			if isHardStop(err) {
				f.hardStop(err)
			}
		})
	}

	// Log each redial of a failed connection. Under StopOnDrop refuse it;
	// otherwise count it as a request and charge it a request token, so a
	// redial is seen and counts against the request rate.
	if w, ok := client.(redialWatcher); ok {
		w.OnRedial(func(ctx context.Context, err error) error {
			// Refuse the redial under StopOnDrop.
			if opts.StopOnDrop {
				f.logf(refusedRedialLine, err)
				return errors.Wrap(errRedialRefused, err.Error())
			}

			// Otherwise count it and wait for a request token.
			f.logf(redialLine, err)
			f.requests.Add(1)
			if f.reqs != nil {
				return f.reqs.Wait(ctx)
			}
			return nil
		})
	}

	// Build the rate limiters the options ask for.
	if opts.RequestsPerSec > 0 {
		f.reqs = rate.NewLimiter(rate.Limit(opts.RequestsPerSec), 1)
	}
	if opts.BytesPerSec > 0 {
		f.bytes = rate.NewLimiter(rate.Limit(opts.BytesPerSec), max(int(opts.BytesPerSec), 1<<20))
	}
	return f
}

// logf writes a line to the log, when there is one.
func (f *fetcher) logf(format string, args ...any) {
	if f.opts.Log != nil {
		_, _ = fmt.Fprintf(f.opts.Log, format, args...)
	}
}

// hardStop stops the run at a hard stop: no request starts after it, and
// every session of the client is closed at once, control and data
// connections, which cuts the requests in flight on them. It returns the
// first hard stop's error.
func (f *fetcher) hardStop(err error) *hardStopError {
	f.stopOnce.Do(func() {
		// Set the error before the latch, which publishes it.
		f.stopErr = &hardStopError{err: err}
		f.stopped.Store(true)
		if c, ok := f.client.(sessionCloser); ok {
			c.CloseAll()
		}
	})
	return f.stopErr
}

// stopError returns the first hard stop's error, or nil.
func (f *fetcher) stopError() error {
	if !f.stopped.Load() {
		return nil
	}
	return f.stopErr
}

// reserve counts n bytes against the cap, or returns errCapReached when they
// would cross it.
func (f *fetcher) reserve(n uint64) error {
	for {
		cur := f.fetched.Load()
		if cur+n > f.opts.CapBytes {
			return errCapReached
		}
		if f.fetched.CompareAndSwap(cur, cur+n) {
			return nil
		}
	}
}

// wait takes the rate tokens of a request of n bytes.
func (f *fetcher) wait(ctx context.Context, n uint64) error {
	if f.reqs != nil {
		if err := f.reqs.Wait(ctx); err != nil {
			return err
		}
	}
	if f.bytes != nil {
		for n > 0 {
			step := min(n, uint64(f.bytes.Burst()))               //nolint:gosec // burst is positive.
			if err := f.bytes.WaitN(ctx, int(step)); err != nil { //nolint:gosec // step is at most the burst, an int.
				return err
			}
			n -= step
		}
	}
	return nil
}

// whole reads the whole object of size bytes, reading at most one byte past
// size: a larger object fails with errOverSize.
func (f *fetcher) whole(ctx context.Context, key string, size uint64) ([]byte, error) {
	return f.do(ctx, size, func(ctx context.Context) ([]byte, error) {
		// Read through the client's own limit when it has one.
		if mr, ok := f.client.(objectMaxReader); ok {
			data, err := f.wholeMax(ctx, mr, key, size)
			if errors.Is(err, block_store_ftps.ErrTooLarge) {
				return nil, errors.Wrap(errOverSize, key)
			}
			return data, err
		}

		// Open the object's body.
		body, err := f.client.GetObject(ctx, f.bucket, key)
		if err != nil {
			return nil, err
		}
		defer body.Close()

		// Read it into a buffer of the expected size, and one byte more to
		// tell a larger object.
		var buf bytes.Buffer
		buf.Grow(int(size) + 1)                                     //nolint:gosec // pack sizes fit memory.
		_, err = io.Copy(&buf, io.LimitReader(body, int64(size)+1)) //nolint:gosec // pack sizes fit int64.
		if err == nil && uint64(buf.Len()) > size {                 //nolint:gosec // a length is not negative.
			return nil, errors.Wrap(errOverSize, key)
		}
		return buf.Bytes(), err
	})
}

// wholeMax reads the whole object through the client's limit. With the slow
// check on and a client that has it, a slow transfer whose session is kept
// is asked for again on that session, within this one attempt, so the pack
// is charged against the cap once; each re-ask is logged and takes a
// request token. The last ask has no check, so no pack fails for being slow.
func (f *fetcher) wholeMax(ctx context.Context, mr objectMaxReader, key string, size uint64) ([]byte, error) {
	// Read as before with the check off.
	sr, ok := f.client.(objectSlowReader)
	if !ok || !f.opts.slowOn() {
		return mr.GetObjectMax(ctx, f.bucket, key, int64(size)) //nolint:gosec // pack sizes fit int64.
	}

	// Ask, re-asking a slow transfer whose session is kept.
	id := path.Base(key)
	for n := 0; ; n++ {
		var chk block_store_ftps.SlowCheck
		if n < f.opts.SlowReasks {
			chk = block_store_ftps.SlowCheck{Rate: f.opts.SlowRate, Grace: f.opts.SlowGrace}
		}
		start := time.Now()
		data, err := sr.GetObjectMaxSlow(ctx, f.bucket, key, int64(size), chk) //nolint:gosec // pack sizes fit int64.
		var se *block_store_ftps.SlowError
		if !errors.As(err, &se) || se.Ended {
			// Log the ask that read a re-asked pack whole.
			if err == nil && n != 0 {
				ms := time.Since(start).Milliseconds()
				f.logf(reaskDoneLine, id, n+1, len(data), ms, int64(len(data))*1000/max(ms, 1))
			}
			return data, err
		}

		// Log the re-ask and wait for its request token.
		f.logf(reaskLine, n+1, f.opts.SlowReasks, id, se.Bytes, se.Elapsed.Milliseconds(), se.Rate(), se.Close)
		if f.stopped.Load() {
			return nil, errHardStopped
		}
		f.requests.Add(1)
		if f.reqs != nil {
			if err := f.reqs.Wait(ctx); err != nil {
				return nil, err
			}
		}
	}
}

// size asks the server for the object's size, with the retries and stops of
// any request. It counts no bytes against the cap.
func (f *fetcher) size(ctx context.Context, key string) (uint64, error) {
	// Find the client's SIZE.
	sizer, ok := f.client.(objectSizer)
	if !ok {
		return 0, errNoSizer
	}

	// Ask for it as a request of no bytes.
	var size int64
	_, err := f.do(ctx, 0, func(ctx context.Context) ([]byte, error) {
		var err error
		size, err = sizer.ObjectSize(ctx, f.bucket, key)
		return nil, err
	})
	if err == nil && size < 0 {
		err = errors.Errorf("negative size %d", size)
	}
	return uint64(size), err //nolint:gosec // checked not negative.
}

// rangeRead reads length bytes of the object at off.
func (f *fetcher) rangeRead(ctx context.Context, key string, off int64, length int) ([]byte, error) {
	return f.do(ctx, uint64(length), func(ctx context.Context) ([]byte, error) { //nolint:gosec // lengths are not negative.
		return f.client.GetObjectRange(ctx, f.bucket, key, off, length)
	})
}

// do runs a request of n bytes with retries. A hard stop returns a
// *hardStopError at once; the cap returns errCapReached before the request.
func (f *fetcher) do(ctx context.Context, n uint64, fn func(context.Context) ([]byte, error)) ([]byte, error) {
	backoff := f.opts.Backoff
	for attempt := 0; ; attempt++ {
		// Start nothing after a hard stop, then count the bytes against the
		// cap and wait for the rate.
		if f.stopped.Load() {
			return nil, errHardStopped
		}
		if err := f.reserve(n); err != nil {
			return nil, err
		}
		if n != 0 && f.opts.Reserved != nil {
			if err := f.opts.Reserved(f.fetched.Load()); err != nil {
				return nil, errors.Wrap(err, "record the byte reservation")
			}
		}
		if err := f.wait(ctx, n); err != nil {
			return nil, err
		}

		// Make the attempt, unless a hard stop came while it waited.
		if f.stopped.Load() {
			return nil, errHardStopped
		}
		// A request in flight when a hard stop came was cut: drop what it
		// read, even if it finished first. Its bytes stay counted.
		f.requests.Add(1)
		st, _ := ctx.Value(reqStatsKey{}).(*reqStats)
		st.begin()
		data, err := f.attempt(ctx, fn)
		st.end()
		if f.opts.StopOnDrop && ctx.Err() == nil && !f.stopped.Load() && errors.Is(err, block_store_ftps.ErrSlowEnded) {
			f.logf(slowEndLine, err)
			return nil, f.hardStop(err)
		}
		if f.opts.StopOnDrop && ctx.Err() == nil && !f.stopped.Load() && !isHardStop(err) && isDrop(err) {
			f.logf(dropLine, err)
			return nil, f.hardStop(err)
		}
		if isHardStop(err) {
			return nil, f.hardStop(err)
		}
		if f.stopped.Load() {
			return nil, errCut
		}
		if err == nil {
			return data, nil
		}
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		if !isRetryable(err) || attempt >= f.opts.Retries {
			return nil, err
		}

		// Back off before the next attempt.
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		}
		backoff = min(2*backoff, max(f.opts.MaxBackoff, f.opts.Backoff))
	}
}

// attempt runs fn bounded by the timeout and waits for it to return. The
// FTPS client closes a session whose call outlives its context
// (Config.CloseOnDone), so fn returns once the session's sockets are closed:
// a timed out request never leaves a session open behind it.
func (f *fetcher) attempt(ctx context.Context, fn func(context.Context) ([]byte, error)) ([]byte, error) {
	// Run fn directly when there is no timeout.
	if f.opts.Timeout <= 0 {
		return fn(ctx)
	}

	// Run fn under the timeout, telling a timeout from the run's cancel.
	actx, cancel := context.WithTimeout(ctx, f.opts.Timeout)
	defer cancel()
	data, err := fn(actx)
	if err != nil && actx.Err() != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, errTimeout
	}
	return data, err
}

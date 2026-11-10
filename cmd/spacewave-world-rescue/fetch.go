//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"context"
	"io"
	"net/textproto"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
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

// errCapReached reports that the next request would cross the byte cap.
var errCapReached = errors.New("byte cap reached")

// errTimeout reports that a request outlived its timeout.
var errTimeout = errors.New("request timed out")

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
	if errors.Is(err, block_store_s3.ErrNotFound) || errors.Is(err, context.Canceled) {
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
}

// newFetcher builds a fetcher whose byte count starts at fetched.
func newFetcher(client objectReader, bucket string, opts fetchOptions, fetched uint64) *fetcher {
	// Start the byte count at the ledger's total.
	f := &fetcher{client: client, bucket: bucket, opts: opts}
	f.fetched.Store(fetched)

	// Build the rate limiters the options ask for.
	if opts.RequestsPerSec > 0 {
		f.reqs = rate.NewLimiter(rate.Limit(opts.RequestsPerSec), 1)
	}
	if opts.BytesPerSec > 0 {
		f.bytes = rate.NewLimiter(rate.Limit(opts.BytesPerSec), max(int(opts.BytesPerSec), 1<<20))
	}
	return f
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

// whole reads the whole object of size bytes.
func (f *fetcher) whole(ctx context.Context, key string, size uint64) ([]byte, error) {
	return f.do(ctx, size, func(ctx context.Context) ([]byte, error) {
		// Open the object's body.
		body, err := f.client.GetObject(ctx, f.bucket, key)
		if err != nil {
			return nil, err
		}
		defer body.Close()

		// Read it into a buffer of the expected size.
		var buf bytes.Buffer
		buf.Grow(int(size)) //nolint:gosec // pack sizes fit memory.
		_, err = io.Copy(&buf, body)
		return buf.Bytes(), err
	})
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
		// Count the bytes against the cap and wait for the rate.
		if err := f.reserve(n); err != nil {
			return nil, err
		}
		if err := f.wait(ctx, n); err != nil {
			return nil, err
		}

		// Make the attempt.
		f.requests.Add(1)
		data, err := f.attempt(ctx, fn)
		if err == nil {
			return data, nil
		}
		if isHardStop(err) {
			return nil, &hardStopError{err: err}
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

// attempt runs fn bounded by the timeout. The FTP commands do not take a
// context, so a timed out attempt is abandoned: it keeps its session until
// the server answers, and its result is dropped.
func (f *fetcher) attempt(ctx context.Context, fn func(context.Context) ([]byte, error)) ([]byte, error) {
	// Run fn directly when there is no timeout.
	if f.opts.Timeout <= 0 {
		return fn(ctx)
	}

	// Run fn in its own goroutine under the timeout.
	actx, cancel := context.WithTimeout(ctx, f.opts.Timeout)
	defer cancel()
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := fn(actx)
		ch <- result{data, err}
	}()

	// Return its result, or a timeout or the run's cancel, whichever is first.
	select {
	case r := <-ch:
		if r.err != nil && actx.Err() != nil && ctx.Err() == nil {
			return nil, errTimeout
		}
		return r.data, r.err
	case <-actx.Done():
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, errTimeout
	}
}

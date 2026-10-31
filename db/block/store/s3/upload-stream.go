//go:build !tinygo

package block_store_s3

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

// UploadStream defaults.
const (
	// DefaultPartSize is the part size of UploadStream. At MaxParts parts it
	// carries a stream of up to 625 GiB.
	DefaultPartSize = 64 << 20
	// DefaultUploadConcurrency is the number of parts UploadStream sends at
	// once.
	DefaultUploadConcurrency = 16
)

// partRetryDelays are the waits before each retry of a part whose request
// failed after the client's own retries. A long upload meets more passing
// faults than one request, so these waits are longer than retryDelays.
var partRetryDelays = []time.Duration{
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	time.Minute,
}

// abortTimeout bounds the abort of a failed or cancelled upload.
const abortTimeout = time.Minute

// UploadStreamOptions configures UploadStream. The zero value uses the
// defaults.
type UploadStreamOptions struct {
	// PartSize is the size of each part but the last, from MinPartSize to
	// MaxPartSize. Zero uses DefaultPartSize. The stream may be at most
	// MaxParts parts long, so a stream over 625 GiB needs a larger part.
	PartSize int
	// Concurrency is the number of parts uploaded at once. Zero uses
	// DefaultUploadConcurrency. The upload holds Concurrency+1 part buffers.
	Concurrency int
	// Header holds headers stored with the object, such as Content-Type.
	Header http.Header
	// Progress is called, when set, with the size of each part once the
	// service has stored it. Calls may come from several goroutines at once.
	Progress func(n int)
}

// UploadStream uploads r to key without knowing its length beforehand, as for
// a pipe. A stream shorter than one part is one PutObject. A longer stream is
// a multipart upload of opts.PartSize parts, opts.Concurrency at a time, with
// each part retried on passing faults; on any error or cancellation the
// upload is aborted, so no partial object or stored parts remain. Returns the
// number of bytes uploaded.
func (c *Client) UploadStream(ctx context.Context, bucket, key string, r io.Reader, opts *UploadStreamOptions) (int64, error) {
	// Resolve the options.
	var o UploadStreamOptions
	if opts != nil {
		o = *opts
	}
	if o.PartSize == 0 {
		o.PartSize = DefaultPartSize
	}
	if o.Concurrency <= 0 {
		o.Concurrency = DefaultUploadConcurrency
	}
	if o.PartSize < MinPartSize || o.PartSize > MaxPartSize {
		return 0, errors.Errorf("part size %d is outside %d to %d", o.PartSize, MinPartSize, MaxPartSize)
	}

	// Read the first part, and store a stream that fits in it as one object.
	first := make([]byte, o.PartSize)
	n, err := io.ReadFull(r, first)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		if err := c.PutObjectHeader(ctx, bucket, key, first[:n], o.Header); err != nil {
			return 0, err
		}
		if o.Progress != nil {
			o.Progress(n)
		}
		return int64(n), nil
	}
	if err != nil {
		return 0, errors.Wrap(err, "read stream")
	}

	// Start the multipart upload.
	uploadID, err := c.CreateMultipartUpload(ctx, bucket, key, o.Header)
	if err != nil {
		return 0, err
	}

	// Upload the parts, aborting the upload when any step fails. The abort
	// runs even when ctx is cancelled.
	total, err := c.uploadParts(ctx, bucket, key, uploadID, r, first, &o)
	if err != nil {
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), abortTimeout)
		defer cancel()
		if aerr := c.AbortMultipartUpload(abortCtx, bucket, key, uploadID); aerr != nil {
			return 0, errors.Wrapf(err, "abort upload %s also failed: %v", uploadID, aerr)
		}
		return 0, err
	}
	return total, nil
}

// uploadParts reads r into parts after first, uploads them through the
// upload, and completes it with the ordered part list. It holds at most
// Concurrency+1 part buffers: Concurrency uploading and one filling.
func (c *Client) uploadParts(ctx context.Context, bucket, key, uploadID string, r io.Reader, first []byte, o *UploadStreamOptions) (int64, error) {
	// Run the part uploads under one group that stops on the first failure.
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(o.Concurrency)
	free := make(chan []byte, o.Concurrency+1)
	allocated := 1
	etags := make([]*string, 0, 64)
	var total int64

	// Read and send each part in order until the stream ends.
	buf, n := first, len(first)
	for partNumber := 1; n != 0; partNumber++ {
		// Refuse a stream longer than the upload can hold.
		if partNumber > MaxParts {
			_ = eg.Wait()
			return 0, errors.Errorf("stream exceeds %d parts of %d bytes; raise the part size", MaxParts, o.PartSize)
		}

		// Send the part into its own ETag slot, returning its buffer to the
		// free list once stored.
		etag := new(string)
		etags = append(etags, etag)
		total += int64(n)
		part, number := buf[:n], partNumber
		eg.Go(func() error {
			// Upload the part, recording its ETag and progress.
			defer func() { free <- part[:cap(part)] }()
			var err error
			*etag, err = c.uploadPartRetry(egCtx, bucket, key, uploadID, number, part)
			if err != nil {
				return err
			}
			if o.Progress != nil {
				o.Progress(len(part))
			}
			return nil
		})

		// Take a free buffer, or allocate one while under the limit.
		switch {
		case allocated < o.Concurrency+1:
			buf = make([]byte, o.PartSize)
			allocated++
		default:
			select {
			case buf = <-free:
			case <-egCtx.Done():
				if err := eg.Wait(); err != nil {
					return 0, err
				}
				return 0, context.Cause(egCtx)
			}
		}

		// Fill it with the next part; a short read is the last part.
		var err error
		n, err = io.ReadFull(r, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			_ = eg.Wait()
			return 0, errors.Wrap(err, "read stream")
		}
	}

	// Wait for the parts, then join them in order.
	if err := eg.Wait(); err != nil {
		return 0, err
	}
	parts := make([]CompletedPart, len(etags))
	for i, etag := range etags {
		parts[i] = CompletedPart{PartNumber: i + 1, ETag: *etag}
	}
	if err := c.CompleteMultipartUpload(ctx, bucket, key, uploadID, parts); err != nil {
		return 0, err
	}
	return total, nil
}

// uploadPartRetry uploads one part, retrying with partRetryDelays while the
// failure is a passing fault.
func (c *Client) uploadPartRetry(ctx context.Context, bucket, key, uploadID string, partNumber int, data []byte) (string, error) {
	for attempt := 0; ; attempt++ {
		// Send the part and return unless the failure may pass.
		etag, err := c.UploadPart(ctx, bucket, key, uploadID, partNumber, data)
		if err == nil || attempt == len(partRetryDelays) || ctx.Err() != nil || !isTransientError(err) {
			return etag, err
		}

		// Wait before the next attempt.
		timer := time.NewTimer(partRetryDelays[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", context.Cause(ctx)
		case <-timer.C:
		}
	}
}

// isTransientError reports whether err is a failure in transit or a status
// the service returns for a passing fault or overload.
func isTransientError(err error) bool {
	var serr *StatusError
	if !errors.As(err, &serr) {
		return !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrBucketNotFound)
	}
	return serr.StatusCode == http.StatusTooManyRequests || serr.StatusCode/100 == 5
}

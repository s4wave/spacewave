//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"

	"github.com/pkg/errors"
)

// MinPartBytes is the smallest part the S3 API accepts, apart from the last.
const MinPartBytes = 5 << 20

// maxParts is the most parts one object may have.
const maxParts = 10000

// StreamOptions configures PutObjectStream.
type StreamOptions struct {
	// ContentType is the object's type.
	ContentType string
	// PartBytes is the size of each part, at least MinPartBytes. The largest
	// object is 10,000 of them.
	PartBytes int
	// Workers is the parts uploaded at once, which is what fills a fast link.
	// Memory use is Workers parts plus one.
	Workers int
}

// PutObjectStream uploads r as one object without holding it whole, and
// returns its size. A stream shorter than a part goes up as one PUT; a longer
// one goes as a multipart upload with parts in flight together. The object
// appears only when every part has landed. On failure the upload is aborted
// and no object appears.
func (c *Client) PutObjectStream(ctx context.Context, bucket, key string, r io.Reader, opts StreamOptions) (int64, error) {
	// Read the first part. A stream that ends inside it needs no multipart.
	if opts.PartBytes < MinPartBytes {
		return 0, errors.Errorf("part of %d bytes is under the %d byte minimum", opts.PartBytes, MinPartBytes)
	}
	first, err := readPart(r, opts.PartBytes)
	if err != nil {
		return 0, err
	}
	if len(first) < opts.PartBytes {
		return int64(len(first)), c.PutObject(ctx, bucket, key, first, opts.ContentType)
	}

	// Begin the upload and abort it unless it completes.
	uploadID, err := c.createMultipart(ctx, bucket, key, opts.ContentType)
	if err != nil {
		return 0, err
	}
	size, parts, err := c.uploadParts(ctx, bucket, key, uploadID, first, r, opts)
	if err == nil {
		err = c.completeMultipart(ctx, bucket, key, uploadID, parts)
	}
	if err != nil {
		c.abortMultipart(context.WithoutCancel(ctx), bucket, key, uploadID)
		return 0, err
	}
	return size, nil
}

// readPart reads up to n bytes, returning fewer only at the end of r.
func readPart(r io.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
	read, err := io.ReadFull(r, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return buf[:read], nil
	}
	return buf, err
}

// uploadParts sends first and the rest of r, opts.Workers parts at a time, and
// returns the size and the completed parts in order.
func (c *Client) uploadParts(
	ctx context.Context,
	bucket, key, uploadID string,
	first []byte,
	r io.Reader,
	opts StreamOptions,
) (int64, []completedPart, error) {
	// Stop every part when one fails.
	ctx, stop := context.WithCancelCause(ctx)
	defer stop(nil)

	// Keep the landed parts as the workers finish.
	var wg sync.WaitGroup
	var mtx sync.Mutex
	var parts []completedPart

	// Hand each part to a worker, holding a slot while it uploads.
	slots := make(chan struct{}, max(opts.Workers, 1))
	var size int64
	var readErr error
	next := first
	for number := 1; len(next) != 0; number++ {
		// Refuse an object too large for the part size.
		if number > maxParts {
			readErr = errors.Errorf("object exceeds %d parts of %d bytes", maxParts, opts.PartBytes)
			break
		}

		// Wait for a slot, or stop if a part failed.
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}

		// Start the part. The worker keeps its own bytes, since next moves on.
		data := next
		size += int64(len(data))
		wg.Go(func() {
			// Upload the part, stopping the rest on failure.
			defer func() { <-slots }()
			etag, err := c.uploadPart(ctx, bucket, key, uploadID, number, data)
			if err != nil {
				stop(err)
				return
			}

			// Record it.
			mtx.Lock()
			parts = append(parts, completedPart{Number: number, ETag: etag})
			mtx.Unlock()
		})

		// Read the next part while this one uploads. A short part was the last.
		if len(data) < opts.PartBytes {
			break
		}
		if next, readErr = readPart(r, opts.PartBytes); readErr != nil {
			break
		}
	}
	wg.Wait()

	// Report the first failure, and the parts in order.
	if readErr != nil {
		return 0, nil, readErr
	}
	if err := context.Cause(ctx); err != nil {
		return 0, nil, err
	}
	slices.SortFunc(parts, func(a, b completedPart) int { return a.Number - b.Number })
	return size, parts, nil
}

// createMultipart begins an upload and returns its id.
func (c *Client) createMultipart(ctx context.Context, bucket, key, contentType string) (string, error) {
	// Ask for the upload.
	header := http.Header{"Content-Type": {contentType}}
	resp, err := c.do(ctx, http.MethodPost, bucket, key, url.Values{"uploads": {""}}, nil, header)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, bucket, key, http.MethodPost); err != nil {
		return "", err
	}

	// Read its id.
	var result struct {
		// UploadID names the upload.
		UploadID string `xml:"UploadId"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errors.Wrap(err, "read upload id")
	}
	return result.UploadID, nil
}

// uploadPart sends one part and returns its ETag.
func (c *Client) uploadPart(ctx context.Context, bucket, key, uploadID string, number int, data []byte) (string, error) {
	// Send the part.
	query := url.Values{"partNumber": {strconv.Itoa(number)}, "uploadId": {uploadID}}
	resp, err := c.do(ctx, http.MethodPut, bucket, key, query, data, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Take its tag from a success.
	if err := checkStatus(resp, bucket, key, http.MethodPut); err != nil {
		return "", err
	}
	return resp.Header.Get("ETag"), nil
}

// completedPart is one landed part, as the completion request lists it.
type completedPart struct {
	// Number is the part's position, from 1.
	Number int `xml:"PartNumber"`
	// ETag is the tag the service gave the part.
	ETag string `xml:"ETag"`
}

// completeMultipart joins the parts into the object.
func (c *Client) completeMultipart(ctx context.Context, bucket, key, uploadID string, parts []completedPart) error {
	// List the parts.
	body, err := xml.Marshal(struct {
		XMLName xml.Name        `xml:"CompleteMultipartUpload"`
		Parts   []completedPart `xml:"Part"`
	}{Parts: parts})
	if err != nil {
		return err
	}

	// Ask for the join.
	resp, err := c.do(ctx, http.MethodPost, bucket, key, url.Values{"uploadId": {uploadID}}, body, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, bucket, key, http.MethodPost); err != nil {
		return err
	}

	// The service may report a failed join in the body of a success.
	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "read completion result")
	}
	if bytes.Contains(result, []byte("<Error>")) {
		return errors.Errorf("complete upload of %s: %s", key, result)
	}
	return nil
}

// abortMultipart drops an upload's parts. A failure leaves them for the
// bucket's lifecycle rule, so it is not reported.
func (c *Client) abortMultipart(ctx context.Context, bucket, key, uploadID string) {
	resp, err := c.do(ctx, http.MethodDelete, bucket, key, url.Values{"uploadId": {uploadID}}, nil, nil)
	if err == nil {
		_ = resp.Body.Close()
	}
}

//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Multipart upload limits shared by S3 and Backblaze B2.
const (
	// MinPartSize is the smallest part except the last.
	MinPartSize = 5 << 20
	// MaxPartSize is the largest part.
	MaxPartSize = 5 << 30
	// MaxParts is the most parts one upload may have.
	MaxParts = 10000
)

// CompletedPart is one uploaded part of a multipart upload.
type CompletedPart struct {
	// PartNumber is the part's position, starting at 1.
	PartNumber int
	// ETag is the tag the service returned for the part.
	ETag string
}

// CreateMultipartUpload starts a multipart upload of key with the headers the
// object will carry, and returns its upload ID.
func (c *Client) CreateMultipartUpload(ctx context.Context, bucket, key string, header http.Header) (string, error) {
	// Start the upload.
	resp, err := c.do(ctx, http.MethodPost, bucket, key, url.Values{"uploads": {""}}, nil, header)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, bucket, key, http.MethodPost); err != nil {
		return "", err
	}

	// Read the upload ID from the result.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "read create multipart upload result")
	}
	uploadID := html.UnescapeString(xmlElementText(string(body), "UploadId"))
	if uploadID == "" {
		return "", errors.Errorf("create multipart upload %s/%s: no upload ID in result", bucket, key)
	}
	return uploadID, nil
}

// UploadPart stores data as part partNumber, from 1 to MaxParts, of the upload
// and returns its ETag. Uploading the same part number again replaces it.
func (c *Client) UploadPart(ctx context.Context, bucket, key, uploadID string, partNumber int, data []byte) (string, error) {
	// Send the part.
	query := url.Values{"partNumber": {strconv.Itoa(partNumber)}, "uploadId": {uploadID}}
	resp, err := c.do(ctx, http.MethodPut, bucket, key, query, data, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, bucket, key, http.MethodPut); err != nil {
		return "", err
	}

	// Return the part's ETag.
	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", errors.Errorf("upload part %d of %s/%s: no ETag in response", partNumber, bucket, key)
	}
	return etag, nil
}

// CompleteMultipartUpload joins the parts, in part number order, into the
// object.
func (c *Client) CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, parts []CompletedPart) error {
	// Write the part list.
	var b strings.Builder
	b.WriteString("<CompleteMultipartUpload>")
	for _, part := range parts {
		b.WriteString("<Part><PartNumber>")
		b.WriteString(strconv.Itoa(part.PartNumber))
		b.WriteString("</PartNumber><ETag>")
		b.WriteString(html.EscapeString(part.ETag))
		b.WriteString("</ETag></Part>")
	}
	b.WriteString("</CompleteMultipartUpload>")

	// Send it.
	query := url.Values{"uploadId": {uploadID}}
	header := http.Header{"Content-Type": {"application/xml"}}
	resp, err := c.do(ctx, http.MethodPost, bucket, key, query, []byte(b.String()), header)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, bucket, key, http.MethodPost); err != nil {
		return err
	}

	// The service may report a failed join in the body of a successful
	// response.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "read complete multipart upload result")
	}
	if bytes.Contains(body, []byte("<Error>")) {
		return errors.Errorf("complete multipart upload %s/%s: %s", bucket, key, body)
	}
	return nil
}

// AbortMultipartUpload discards the upload and the parts stored for it. A
// missing upload is already aborted.
func (c *Client) AbortMultipartUpload(ctx context.Context, bucket, key, uploadID string) error {
	// Abort the upload.
	resp, err := c.do(ctx, http.MethodDelete, bucket, key, url.Values{"uploadId": {uploadID}}, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Treat a missing upload as aborted.
	err = checkStatus(resp, bucket, key, http.MethodDelete)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

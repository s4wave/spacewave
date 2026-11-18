//go:build !tinygo

package block_store_s3

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// errColdObjectMismatch reports evidence that the bucket holds a different pack.
var errColdObjectMismatch = errors.New("cold pack does not match manifest")

// GetColdPackRange reads an exact range of a pack with a known object size.
// It returns an optional service-verified whole-object SHA-256, never an ETag
// or a checksum of multipart checksums or of just the returned range.
func (c *Client) GetColdPackRange(ctx context.Context, bucket, key string, off int64, length int, size uint64) ([]byte, []byte, error) {
	// Reject a range outside the known pack or the reader's byte bound.
	if off < 0 || length <= 0 || length == math.MaxInt || size > math.MaxInt64 || uint64(off) >= size || uint64(length) > size-uint64(off) {
		return nil, nil, errors.New("cold range is outside the manifest's pack size")
	}

	// Keep the pack's binding on the same response as its bytes.
	span := strconv.FormatInt(off, 10) + "-" + strconv.FormatInt(off+int64(length)-1, 10)
	header := http.Header{"Range": {"bytes=" + span}, "X-Amz-Checksum-Mode": {"ENABLED"}}
	resp, err := c.do(ctx, http.MethodGet, bucket, key, nil, nil, header)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	// Compare the whole object length before using any range bytes.
	contentRange := resp.Header.Get("Content-Range")
	spanHeader, total, hasTotal := strings.Cut(contentRange, "/")
	if resp.StatusCode == http.StatusPartialContent || resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		if n, err := strconv.ParseUint(total, 10, 64); hasTotal && err == nil && n != size {
			return nil, nil, errColdObjectMismatch
		}
	}
	if err := checkStatus(resp, bucket, key, http.MethodGet); err != nil {
		return nil, nil, err
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		if spanHeader != "bytes "+span || total != strconv.FormatUint(size, 10) {
			return nil, nil, errors.New("cold pack returned an invalid Content-Range")
		}
	case http.StatusOK:
		if resp.ContentLength >= 0 && uint64(resp.ContentLength) != size {
			return nil, nil, errColdObjectMismatch
		}
		if off != 0 || uint64(length) != size {
			return nil, nil, errors.New("server ignored cold pack range")
		}
	default:
		return nil, nil, errors.New("unexpected cold pack response status")
	}

	// Keep the requested byte bound even if the server sends a bad body.
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(length)+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) != length {
		return nil, nil, errColdInvalidExtent
	}
	if off == 0 && uint64(length) == size {
		digest := sha256.Sum256(data)
		return data, digest[:], nil
	}

	// Older uploads can lack a full checksum. Their ranges can locate and
	// verify individual blocks, but their tails cannot prove pack membership.
	if resp.Header.Get("X-Amz-Checksum-Type") == "FULL_OBJECT" {
		sum, err := base64.StdEncoding.DecodeString(resp.Header.Get(ChecksumSHA256Header))
		if err == nil && len(sum) == sha256.Size {
			return data, sum, nil
		}
	}
	return data, nil, nil
}

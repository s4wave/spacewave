//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"encoding/base64"
	"html"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/httpclient"
)

// emptyPayloadHash is the hex sha256 of an empty body.
const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// Client is a minimal HTTP client for an S3-compatible API.
// Supports GET/HEAD/PUT/DELETE on objects, object and version listing, with
// AWS SigV4 signing.
type Client struct {
	httpClient *http.Client
	endpoint   string
	region     string
	accessKey  string
	secretKey  string
	token      string
	useSSL     bool
}

// BuildClient constructs an S3 client from the config.
func BuildClient(conf *ClientConfig) (*Client, error) {
	region := conf.GetRegion()
	if region == "" {
		region = "us-east-1"
	}
	creds := conf.GetCredentials()
	return &Client{
		httpClient: newHTTPClient(),
		endpoint:   strings.TrimSuffix(conf.GetEndpoint(), "/"),
		region:     region,
		accessKey:  creds.GetAccessKeyId(),
		secretKey:  creds.GetSecretAccessKey(),
		token:      creds.GetToken(),
		useSSL:     !conf.GetDisableSsl(),
	}, nil
}

// PutObject uploads an object with the given content type.
func (c *Client) PutObject(ctx context.Context, bucket, key string, data []byte, contentType string) error {
	resp, err := c.do(ctx, http.MethodPut, bucket, key, nil, data, http.Header{"Content-Type": {contentType}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp, bucket, key, http.MethodPut)
}

// GetObject downloads an object body. Caller must Close the returned reader.
// Returns ErrNotFound if the object does not exist.
func (c *Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	resp, err := c.do(ctx, http.MethodGet, bucket, key, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := checkStatus(resp, bucket, key, http.MethodGet); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	return resp.Body, nil
}

// GetObjectRange reads length bytes of an object from offset off. The result
// is shorter than length when the object ends first.
// Returns ErrNotFound if the object does not exist.
func (c *Client) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	rng := "bytes=" + strconv.FormatInt(off, 10) + "-" + strconv.FormatInt(off+int64(length)-1, 10)
	resp, err := c.do(ctx, http.MethodGet, bucket, key, nil, nil, http.Header{"Range": {rng}})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		return nil, nil
	}
	if err := checkStatus(resp, bucket, key, http.MethodGet); err != nil {
		return nil, err
	}
	// A server may ignore the range and send the whole object.
	if resp.StatusCode != http.StatusPartialContent {
		if _, err := io.CopyN(io.Discard, resp.Body, off); err != nil {
			if err == io.EOF {
				return nil, nil
			}
			return nil, err
		}
	}
	return io.ReadAll(io.LimitReader(resp.Body, int64(length)))
}

// ObjectStat describes a stored object.
type ObjectStat struct {
	// Size is the content length in bytes.
	Size int64
	// ChecksumSHA256 is the SHA-256 the service verified when the object was
	// uploaded with one, or nil.
	ChecksumSHA256 []byte
}

// HeadObject returns the object's size and upload checksum.
// Returns ErrNotFound if the object does not exist.
func (c *Client) HeadObject(ctx context.Context, bucket, key string) (*ObjectStat, error) {
	// Ask for the object's metadata with its stored checksum.
	resp, err := c.do(ctx, http.MethodHead, bucket, key, nil, nil, http.Header{"X-Amz-Checksum-Mode": {"ENABLED"}})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, bucket, key, http.MethodHead); err != nil {
		return nil, err
	}

	// Read the size and decode the checksum when the service reports one.
	stat := &ObjectStat{Size: resp.ContentLength}
	if sum := resp.Header.Get(ChecksumSHA256Header); sum != "" {
		stat.ChecksumSHA256, err = base64.StdEncoding.DecodeString(sum)
		if err != nil {
			return nil, errors.Wrap(err, "decode object checksum")
		}
	}
	return stat, nil
}

// ChecksumSHA256Header carries the base64 SHA-256 of an object's body. The
// service refuses an upload whose body does not match it.
const ChecksumSHA256Header = "X-Amz-Checksum-Sha256"

// PresignPut returns a URL that uploads the object without credentials until
// expires from now, and the headers the upload must send. The service refuses
// an upload whose length or SHA-256 differs from size and sum.
func (c *Client) PresignPut(bucket, key string, size int64, sum []byte, expires time.Duration, now time.Time) (string, http.Header) {
	header := http.Header{
		"Content-Length":     {strconv.FormatInt(size, 10)},
		ChecksumSHA256Header: {base64.StdEncoding.EncodeToString(sum)},
	}
	u := c.objectURL(bucket, key, nil)
	c.presignV4(http.MethodPut, u, header, expires, now)
	return u.String(), header
}

// DeleteObject removes every version of an object, so a bucket with
// versioning, such as Backblaze B2, keeps no hidden copy. A missing object is
// already deleted. Where the service does not list versions, or the key may
// not, it deletes the current version alone.
func (c *Client) DeleteObject(ctx context.Context, bucket, key string) error {
	// List the versions, or delete the current version alone where they are
	// not listed.
	versions, err := c.listObjectVersions(ctx, bucket, key)
	var serr *StatusError
	if errors.As(err, &serr) && (serr.StatusCode == http.StatusNotImplemented || serr.StatusCode == http.StatusForbidden) {
		return c.deleteObjectVersion(ctx, bucket, key, "")
	}
	if err != nil {
		return err
	}

	// Delete each version.
	for _, version := range versions {
		if err := c.deleteObjectVersion(ctx, bucket, key, version); err != nil {
			return err
		}
	}
	return nil
}

// deleteObjectVersion removes one version of an object, or the current version
// when version is empty.
func (c *Client) deleteObjectVersion(ctx context.Context, bucket, key, version string) error {
	// Address the version, or the current version when version is empty.
	var query url.Values
	if version != "" {
		query = url.Values{"versionId": {version}}
	}

	// Delete it. A missing version is already deleted.
	resp, err := c.do(ctx, http.MethodDelete, bucket, key, query, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	err = checkStatus(resp, bucket, key, http.MethodDelete)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// listObjectVersions returns the version ids of key, including delete
// markers.
func (c *Client) listObjectVersions(ctx context.Context, bucket, key string) ([]string, error) {
	query := url.Values{"versions": {""}, "prefix": {key}}
	var versions []string
	for {
		// Read one page of the listing.
		resp, err := c.do(ctx, http.MethodGet, bucket, "", query, nil, nil)
		if err != nil {
			return nil, err
		}
		if err := checkStatus(resp, bucket, key, http.MethodGet); err != nil {
			_ = resp.Body.Close()
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}

		// Collect the versions and delete markers of key alone, since the
		// prefix also matches longer keys.
		doc := string(body)
		for _, name := range []string{"Version", "DeleteMarker"} {
			err := visitElements(doc, name, func(version string) error {
				if html.UnescapeString(xmlElementText(version, "Key")) == key {
					versions = append(versions, html.UnescapeString(xmlElementText(version, "VersionId")))
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
		// Continue from the next page, if any.
		if xmlElementText(doc, "IsTruncated") != "true" {
			return versions, nil
		}
		query.Set("key-marker", html.UnescapeString(xmlElementText(doc, "NextKeyMarker")))
		query.Set("version-id-marker", html.UnescapeString(xmlElementText(doc, "NextVersionIdMarker")))
	}
}

// ListObjects calls fn with the key and size of every object under prefix, in
// key order. Each request lists up to 1000 objects.
func (c *Client) ListObjects(ctx context.Context, bucket, prefix string, fn func(key string, size int64) error) error {
	query := url.Values{"list-type": {"2"}, "prefix": {prefix}}
	for {
		resp, err := c.do(ctx, http.MethodGet, bucket, "", query, nil, nil)
		if err != nil {
			return err
		}
		if err := checkStatus(resp, bucket, prefix, http.MethodGet); err != nil {
			_ = resp.Body.Close()
			return err
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return err
		}

		doc := string(body)
		if err := visitListedObjects(doc, fn); err != nil {
			return err
		}
		// Continue from the next page, if any.
		if xmlElementText(doc, "IsTruncated") != "true" {
			return nil
		}
		token := html.UnescapeString(xmlElementText(doc, "NextContinuationToken"))
		if token == "" {
			return errors.New("s3 list: truncated result without a continuation token")
		}
		query.Set("continuation-token", token)
	}
}

// visitListedObjects calls fn with the key and size of each Contents element
// of a ListObjectsV2 result.
func visitListedObjects(doc string, fn func(key string, size int64) error) error {
	return visitElements(doc, "Contents", func(contents string) error {
		key := html.UnescapeString(xmlElementText(contents, "Key"))
		size, err := strconv.ParseInt(xmlElementText(contents, "Size"), 10, 64)
		if err != nil {
			return errors.Wrap(err, "s3 list: object size")
		}
		return fn(key, size)
	})
}

// visitElements calls fn with the content of each element of doc with the
// given name. Object keys cannot contain an element because XML escapes '<'.
func visitElements(doc, name string, fn func(content string) error) error {
	for {
		_, rest, ok := strings.Cut(doc, "<"+name+">")
		if !ok {
			return nil
		}
		content, rest, ok := strings.Cut(rest, "</"+name+">")
		if !ok {
			return errors.New("s3 list: unterminated " + name + " element")
		}
		if err := fn(content); err != nil {
			return err
		}
		doc = rest
	}
}

// retryDelays are the waits before each retry of a request that failed in
// transit or with a transient status. Every request the client sends is
// idempotent: object writes are content addressed.
var retryDelays = []time.Duration{
	250 * time.Millisecond,
	time.Second,
	3 * time.Second,
}

// do builds, signs, and sends an S3 request, retrying transient failures. An
// empty key addresses the bucket. query, data, and header may be nil.
func (c *Client) do(ctx context.Context, method, bucket, key string, query url.Values, data []byte, header http.Header) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := c.send(ctx, method, bucket, key, query, data, header)
		if attempt == len(retryDelays) || ctx.Err() != nil || !isTransient(resp, err) {
			return resp, err
		}
		if resp != nil {
			httpclient.DrainAndCloseResponseBody(resp)
		}

		timer := time.NewTimer(retryDelays[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, context.Cause(ctx)
		case <-timer.C:
		}
	}
}

// send builds, signs, and sends one attempt of an S3 request.
func (c *Client) send(ctx context.Context, method, bucket, key string, query url.Values, data []byte, header http.Header) (*http.Response, error) {
	// Hash the payload the signature covers.
	var body io.Reader
	payloadHash := emptyPayloadHash
	if data != nil {
		body = bytes.NewReader(data)
		payloadHash = hexSHA256(data)
	}

	// Build the request for the object URL.
	req, err := http.NewRequestWithContext(ctx, method, c.objectURL(bucket, key, query).String(), body)
	if err != nil {
		return nil, err
	}
	if data != nil {
		req.ContentLength = int64(len(data))
	}
	maps.Copy(req.Header, header)

	// Sign and send the request.
	c.signV4(req, payloadHash, time.Now())
	return c.httpClient.Do(req)
}

// objectURL returns the path-style URL of an object. An empty key addresses
// the bucket.
func (c *Client) objectURL(bucket, key string, query url.Values) *url.URL {
	// Choose the scheme and the path-style object path.
	scheme := "http"
	if c.useSSL {
		scheme = "https"
	}
	path := "/" + bucket
	if key != "" {
		path += "/" + key
	}

	// Join them with the endpoint and canonical query.
	return &url.URL{
		Scheme:   scheme,
		Host:     c.endpoint,
		Path:     path,
		RawQuery: canonicalQuery(query),
	}
}

// isTransient reports whether a request failed in transit or with a status
// the service returns for a passing fault or overload.
func isTransient(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// checkStatus maps a missing bucket to ErrBucketNotFound, another 404 to
// ErrNotFound, and any other non-success status to a StatusError.
func checkStatus(resp *http.Response, bucket, key, method string) error {
	if resp.StatusCode/100 == 2 {
		return nil
	}
	serr := newStatusError(resp, method, bucket, key)
	if serr.Code == "NoSuchBucket" {
		return errors.Wrap(ErrBucketNotFound, serr.Error())
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	return serr
}

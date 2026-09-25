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
	"time"

	"github.com/pkg/errors"
)

// emptyPayloadHash is the hex sha256 of an empty body.
const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// Client is a minimal HTTP client for an S3-compatible API.
// Supports GET/HEAD/PUT/DELETE on objects and object listing, with AWS SigV4
// signing.
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

// Validate validates the client config.
func (c *ClientConfig) Validate() error {
	if c.GetEndpoint() == "" {
		return errors.New("endpoint cannot be empty")
	}
	return nil
}

// PutObject uploads an object with the given content type.
func (c *Client) PutObject(ctx context.Context, bucket, key string, data []byte, contentType string) error {
	resp, err := c.do(ctx, http.MethodPut, bucket, key, nil, data, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp, bucket, key, http.MethodPut)
}

// GetObject downloads an object body. Caller must Close the returned reader.
// Returns ErrNotFound if the object does not exist.
func (c *Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	resp, err := c.do(ctx, http.MethodGet, bucket, key, nil, nil, "")
	if err != nil {
		return nil, err
	}
	if err := checkStatus(resp, bucket, key, http.MethodGet); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	return resp.Body, nil
}

// HeadObject returns the object's content length.
// Returns ErrNotFound if the object does not exist.
func (c *Client) HeadObject(ctx context.Context, bucket, key string) (int64, error) {
	resp, err := c.do(ctx, http.MethodHead, bucket, key, nil, nil, "")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, bucket, key, http.MethodHead); err != nil {
		return 0, err
	}
	return resp.ContentLength, nil
}

// DeleteObject removes an object. Returns ErrNotFound if it did not exist.
func (c *Client) DeleteObject(ctx context.Context, bucket, key string) error {
	resp, err := c.do(ctx, http.MethodDelete, bucket, key, nil, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp, bucket, key, http.MethodDelete)
}

// SumObjects lists every object under prefix and returns their count and
// total size. Each request lists up to 1000 objects.
func (c *Client) SumObjects(ctx context.Context, bucket, prefix string) (*ObjectUsage, error) {
	usage := &ObjectUsage{}
	query := url.Values{"list-type": {"2"}, "prefix": {prefix}}
	for {
		resp, err := c.do(ctx, http.MethodGet, bucket, "", query, nil, "")
		if err != nil {
			return nil, err
		}
		if err := checkStatus(resp, bucket, prefix, http.MethodGet); err != nil {
			_ = resp.Body.Close()
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}

		doc := string(body)
		if err := addListedSizes(doc, usage); err != nil {
			return nil, err
		}
		if xmlElementText(doc, "IsTruncated") != "true" {
			return usage, nil
		}
		token := html.UnescapeString(xmlElementText(doc, "NextContinuationToken"))
		if token == "" {
			return nil, errors.New("s3 list: truncated result without a continuation token")
		}
		query.Set("continuation-token", token)
	}
}

// addListedSizes adds each Size element of a ListObjectsV2 result to usage.
// Object keys cannot contain the element because XML escapes '<'.
func addListedSizes(doc string, usage *ObjectUsage) error {
	for {
		_, rest, ok := strings.Cut(doc, "<Size>")
		if !ok {
			return nil
		}
		text, rest, ok := strings.Cut(rest, "</Size>")
		if !ok {
			return errors.New("s3 list: unterminated Size element")
		}
		size, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil {
			return errors.Wrap(err, "s3 list: object size")
		}
		usage.Objects++
		usage.Bytes += size
		doc = rest
	}
}

// do builds, signs, and sends an S3 request. An empty key addresses the
// bucket. query and data may be nil.
func (c *Client) do(ctx context.Context, method, bucket, key string, query url.Values, data []byte, contentType string) (*http.Response, error) {
	scheme := "http"
	if c.useSSL {
		scheme = "https"
	}
	path := "/" + bucket
	if key != "" {
		path += "/" + key
	}
	u := &url.URL{
		Scheme:   scheme,
		Host:     c.endpoint,
		Path:     path,
		RawQuery: canonicalQuery(query),
	}

	var body io.Reader
	payloadHash := emptyPayloadHash
	if data != nil {
		body = bytes.NewReader(data)
		payloadHash = hexSHA256(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if data != nil {
		req.ContentLength = int64(len(data))
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	c.signV4(req, payloadHash, time.Now())
	return c.httpClient.Do(req)
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

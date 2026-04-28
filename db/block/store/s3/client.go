//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// ErrNotFound is returned when an object does not exist on the server.
var ErrNotFound = errors.New("not found")

// emptyPayloadHash is the hex sha256 of an empty body.
const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// Client is a minimal HTTP client for an S3-compatible API.
// Supports basic GET/HEAD/PUT/DELETE on objects with AWS SigV4 signing.
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
		httpClient: http.DefaultClient,
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
	resp, err := c.do(ctx, http.MethodPut, bucket, key, data, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp, bucket, key, http.MethodPut)
}

// GetObject downloads an object body. Caller must Close the returned reader.
// Returns ErrNotFound if the object does not exist.
func (c *Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	resp, err := c.do(ctx, http.MethodGet, bucket, key, nil, "")
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
	resp, err := c.do(ctx, http.MethodHead, bucket, key, nil, "")
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
	resp, err := c.do(ctx, http.MethodDelete, bucket, key, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp, bucket, key, http.MethodDelete)
}

// do builds, signs, and sends an S3 request. data may be nil for read methods.
func (c *Client) do(ctx context.Context, method, bucket, key string, data []byte, contentType string) (*http.Response, error) {
	scheme := "http"
	if c.useSSL {
		scheme = "https"
	}
	u := &url.URL{
		Scheme: scheme,
		Host:   c.endpoint,
		Path:   "/" + bucket + "/" + key,
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

// checkStatus maps an S3 response status to ErrNotFound or a wrapped error.
func checkStatus(resp *http.Response, bucket, key, method string) error {
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode/100 == 2 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return errors.Errorf("s3 %s %s/%s: status %d: %s", method, bucket, key, resp.StatusCode, string(body))
}

//go:build !tinygo

package block_store_s3

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// StatusError is a non-success S3 response.
type StatusError struct {
	// Method is the request method.
	Method string
	// Bucket is the request bucket.
	Bucket string
	// Key is the request object key.
	Key string
	// StatusCode is the HTTP status code.
	StatusCode int
	// Code is the S3 error code, such as AccessDenied. Empty when the
	// response had no error body, as for HEAD.
	Code string
	// Message is the S3 error message.
	Message string
}

// IsTransient reports whether err holds a StatusError with a status the
// service returns for a passing fault or overload, such as a throttled
// bucket's 503 SlowDown, so the same request may succeed later. The client has
// retried such a request before it returns the error, so a caller that sees
// one meets a fault longer than those retries.
func IsTransient(err error) bool {
	var serr *StatusError
	return errors.As(err, &serr) && transientStatus(serr.StatusCode)
}

// transientStatus reports whether status is one the service returns for a
// passing fault or overload.
func transientStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// newStatusError reads the S3 error document from a non-success response.
//
// The document is XML with Code and Message elements. Reading only those two
// elements avoids a reflective XML decoder and never copies other fields, such
// as the StringToSign a signature mismatch echoes.
func newStatusError(resp *http.Response, method, bucket, key string) *StatusError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return &StatusError{
		Method:     method,
		Bucket:     bucket,
		Key:        key,
		StatusCode: resp.StatusCode,
		Code:       xmlElementText(string(body), "Code"),
		Message:    xmlElementText(string(body), "Message"),
	}
}

// Error returns the status, code, and message without request credentials.
func (e *StatusError) Error() string {
	msg := "s3 " + e.Method + " " + e.Bucket + "/" + e.Key + ": status " + strconv.Itoa(e.StatusCode)
	if e.Code != "" {
		msg += ": " + e.Code
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

// xmlElementText returns the text of the first element with the given name.
func xmlElementText(doc, name string) string {
	// Locate the opening XML element or return an empty value.
	_, rest, ok := strings.Cut(doc, "<"+name+">")
	if !ok {
		return ""
	}

	// Extract the XML element text only when its closing tag exists.
	text, _, ok := strings.Cut(rest, "</"+name+">")
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

// Package reqsig verifies requests signed in the Spacewave request signing
// form: an Ed25519 signature by the peer's key over the method, path, time,
// body hash and signed headers. Every server that accepts signed requests
// verifies them here, so one change to the form reaches them all.
package reqsig

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/net/peer"
)

// ClockSkew bounds how far a signed request's timestamp may be from the
// server's clock.
const ClockSkew = 5 * time.Minute

// ErrInvalidSignature reports a request whose signature does not cover it.
var ErrInvalidSignature = errors.New("invalid request signature")

// Verify returns the peer whose key signed r with body at a time near now,
// for the signing environment env. The caller reads the body, so it chooses
// the size limit; the signature covers r.URL.Path as the server sees it, so
// mount a verifying handler without stripping a path prefix.
func Verify(r *http.Request, body []byte, env string, now time.Time) (peer.ID, error) {
	// Read the claimed peer, its public key and the signing time.
	pid, err := peer.IDB58Decode(r.Header.Get("X-Peer-ID"))
	if err != nil {
		return "", err
	}
	pub, err := pid.ExtractPublicKey()
	if err != nil {
		return "", err
	}
	timestamp, err := strconv.ParseInt(r.Header.Get("X-Timestamp"), 10, 64)
	if err != nil {
		return "", err
	}
	if skew := now.Sub(time.UnixMilli(timestamp)); skew > ClockSkew || skew < -ClockSkew {
		return "", errors.New("request time is out of range")
	}

	// Hash the body, and hold a claimed hash to it.
	digest := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(digest[:])
	if claimed := r.Header.Get("X-Sw-Hash"); claimed != "" && claimed != bodyHash {
		return "", errors.New("request body hash mismatch")
	}

	// Rebuild the signed headers in the sorted order the signer used.
	headers, err := signedHeaders(r)
	if err != nil {
		return "", err
	}

	// Bind the signature to this request's method, path, body and headers.
	payload, err := (&api.SigningPayload{
		EnvPrefix:     env,
		Method:        r.Method,
		Path:          r.URL.Path,
		TimestampMs:   timestamp,
		ContentLength: int64(len(body)),
		BodyHashHex:   bodyHash,
		SignedHeaders: headers,
	}).MarshalVT()
	if err != nil {
		return "", err
	}

	// Check the signature over that payload.
	signature, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Signature"))
	if err != nil {
		return "", err
	}
	if ok, err := pub.Verify(payload, signature); err != nil || !ok {
		return "", ErrInvalidSignature
	}
	return pid, nil
}

// signedHeaders returns the "k=v,k=v" form of the headers X-Signed-Headers
// names, which must be sorted and distinct.
func signedHeaders(r *http.Request) (string, error) {
	// Read the named list; no list signs no header.
	list := r.Header.Get("X-Signed-Headers")
	if list == "" {
		return "", nil
	}
	keys := strings.Split(list, ",")
	if !slices.IsSorted(keys) || len(slices.Compact(slices.Clone(keys))) != len(keys) {
		return "", errors.New("signed headers are not sorted and distinct")
	}
	var out strings.Builder
	for i, k := range keys {
		if i > 0 {
			out.WriteByte(',')
		}
		out.WriteString(k)
		out.WriteByte('=')
		out.WriteString(r.Header.Get(k))
	}
	return out.String(), nil
}

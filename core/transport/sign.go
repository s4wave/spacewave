package transport

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// SignHTTPRequest signs an HTTP request using the Spacewave SigningPayload
// proto binary + Ed25519 wire format, at the local clock's time.
func SignHTTPRequest(req *http.Request, body []byte, priv bifrost_crypto.PrivKey, pid peer.ID, envPfx string) error {
	return signHTTPRequestAt(req, body, priv, pid, envPfx, time.Now())
}

// signHTTPRequestAt signs req as SignHTTPRequest does, claiming the time now.
func signHTTPRequestAt(req *http.Request, body []byte, priv bifrost_crypto.PrivKey, pid peer.ID, envPfx string, now time.Time) error {
	// Prepare the request environment, body digest, and signing timestamp.
	if envPfx == "" {
		envPfx = "spacewave"
	}
	h := sha256.Sum256(body)
	bodyHashHex := hex.EncodeToString(h[:])
	timestampMs := now.UnixMilli()

	// Encode the request metadata in the canonical signing payload.
	payload := &api.SigningPayload{
		EnvPrefix:     envPfx,
		Method:        req.Method,
		Path:          req.URL.Path,
		TimestampMs:   timestampMs,
		ContentLength: int64(len(body)),
		BodyHashHex:   bodyHashHex,
	}
	payloadBytes, err := payload.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal signing payload")
	}

	// Sign the encoded request payload with the session private key.
	sig, err := priv.Sign(payloadBytes)
	if err != nil {
		return errors.Wrap(err, "sign payload")
	}

	// Attach the peer identity and signature headers to the HTTP request.
	req.Header.Set("X-Peer-ID", pid.String())
	req.Header.Set("X-Timestamp", strconv.FormatInt(timestampMs, 10))
	req.Header.Set("X-Sw-Hash", bodyHashHex)
	req.Header.Set("X-Signature", base64.StdEncoding.EncodeToString(sig))
	return nil
}

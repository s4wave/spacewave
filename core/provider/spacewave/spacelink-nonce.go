package provider_spacewave

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"time"

	"github.com/pkg/errors"
)

const (
	spaceLinkNonceKeyPrefix = "spacelink-nonce/"
	spaceLinkNonceSkew      = 5 * time.Minute
)

// CheckSpaceLinkNonceFresh performs the PreviewSpaceLink read-only freshness
// check. It never creates or updates the consumed marker.
func (a *ProviderAccount) CheckSpaceLinkNonceFresh(
	ctx context.Context,
	agentPeerID, nonce, payload []byte,
) error {
	return a.checkSpaceLinkNonceFresh(ctx, agentPeerID, nonce, payload, time.Now())
}

func (a *ProviderAccount) checkSpaceLinkNonceFresh(
	ctx context.Context,
	agentPeerID, nonce, payload []byte,
	now time.Time,
) error {
	key, err := spaceLinkNonceKey(agentPeerID, nonce, payload)
	if err != nil {
		return err
	}
	if a.objStore == nil {
		return errors.New("account object store not ready")
	}

	otx, err := a.objStore.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "open read transaction")
	}
	defer otx.Discard()

	data, found, err := otx.Get(ctx, key)
	if err != nil {
		return errors.Wrap(err, "get spacelink nonce marker")
	}
	if !found {
		return nil
	}
	expiresAt, err := parseSpaceLinkNonceMarker(data)
	if err != nil {
		return errors.Wrap(err, "parse spacelink nonce marker")
	}
	if now.Before(expiresAt) {
		return ErrSpaceLinkNonceConsumed
	}
	return nil
}

// ConsumeSpaceLinkNonce atomically records a consumed marker for ApproveSpaceLink.
// The marker is written before cloud registration or SharedObject mutation.
func (a *ProviderAccount) ConsumeSpaceLinkNonce(
	ctx context.Context,
	agentPeerID, nonce, payload []byte,
	expiresAt time.Time,
) error {
	return a.consumeSpaceLinkNonce(ctx, agentPeerID, nonce, payload, expiresAt, time.Now())
}

func (a *ProviderAccount) consumeSpaceLinkNonce(
	ctx context.Context,
	agentPeerID, nonce, payload []byte,
	expiresAt, now time.Time,
) error {
	key, err := spaceLinkNonceKey(agentPeerID, nonce, payload)
	if err != nil {
		return err
	}
	if expiresAt.IsZero() {
		return errors.New("spacelink nonce expiry is required")
	}
	if a.objStore == nil {
		return errors.New("account object store not ready")
	}

	otx, err := a.objStore.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open write transaction")
	}
	defer otx.Discard()

	data, found, err := otx.Get(ctx, key)
	if err != nil {
		return errors.Wrap(err, "get spacelink nonce marker")
	}
	if found {
		markerExpiresAt, err := parseSpaceLinkNonceMarker(data)
		if err != nil {
			return errors.Wrap(err, "parse spacelink nonce marker")
		}
		if now.Before(markerExpiresAt) {
			return ErrSpaceLinkNonceConsumed
		}
	}
	if err := otx.Set(ctx, key, encodeSpaceLinkNonceMarker(expiresAt.Add(spaceLinkNonceSkew))); err != nil {
		return errors.Wrap(err, "set spacelink nonce marker")
	}
	return otx.Commit(ctx)
}

func spaceLinkNonceKey(agentPeerID, nonce, payload []byte) ([]byte, error) {
	if len(agentPeerID) == 0 {
		return nil, errors.New("spacelink agent peer id is required")
	}
	if len(nonce) == 0 {
		return nil, errors.New("spacelink nonce is required")
	}
	if len(payload) == 0 {
		return nil, errors.New("spacelink payload is required")
	}
	payloadDigest := sha256.Sum256(payload)
	key := make([]byte, 0,
		len(spaceLinkNonceKeyPrefix)+
			len("agent=/nonce=/payload=")+
			hex.EncodedLen(len(agentPeerID))+
			hex.EncodedLen(len(nonce))+
			hex.EncodedLen(len(payloadDigest)),
	)
	key = append(key, spaceLinkNonceKeyPrefix...)
	key = append(key, "agent="...)
	key = hex.AppendEncode(key, agentPeerID)
	key = append(key, "/nonce="...)
	key = hex.AppendEncode(key, nonce)
	key = append(key, "/payload="...)
	key = hex.AppendEncode(key, payloadDigest[:])
	return key, nil
}

func encodeSpaceLinkNonceMarker(expiresAt time.Time) []byte {
	var data [8]byte
	binary.BigEndian.PutUint64(data[:], uint64(expiresAt.Unix()))
	return data[:]
}

func parseSpaceLinkNonceMarker(data []byte) (time.Time, error) {
	if len(data) != 8 {
		return time.Time{}, errors.New("invalid spacelink nonce marker")
	}
	secs := int64(binary.BigEndian.Uint64(data))
	return time.Unix(secs, 0), nil
}

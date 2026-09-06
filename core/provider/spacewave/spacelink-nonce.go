package provider_spacewave

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"time"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/db/kvtx"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// CheckSpaceLinkNonceFresh checks account-wide consumption without reserving it.
// Cloud unavailability fails closed; a local cache cannot establish freshness.
func (a *ProviderAccount) CheckSpaceLinkNonceFresh(
	ctx context.Context,
	agentPeerID, nonce, payload []byte,
) error {
	return a.accessSpaceLinkNonce(ctx, agentPeerID, nonce, payload, time.Time{}, false)
}

// ConsumeSpaceLinkNonce durably reserves consent before registration or mutation.
// The cloud account owns replay protection across approver Sessions and caches.
func (a *ProviderAccount) ConsumeSpaceLinkNonce(
	ctx context.Context,
	agentPeerID, nonce, payload []byte,
	expiresAt time.Time,
) error {
	return a.accessSpaceLinkNonce(ctx, agentPeerID, nonce, payload, expiresAt, true)
}

// accessSpaceLinkNonce preserves the verified payload and authenticated account.
func (a *ProviderAccount) accessSpaceLinkNonce(
	ctx context.Context,
	agentPeerID, nonce, payload []byte,
	expiresAt time.Time,
	consume bool,
) error {
	// Never substitute re-encoded consent or mismatched verification inputs.
	var consent s4wave_provider_spacewave.SpaceLinkAuthRequest
	if err := consent.UnmarshalVT(payload); err != nil {
		return errors.Wrap(err, "decode spacelink consent")
	}
	if len(agentPeerID) == 0 || len(nonce) != 16 ||
		!bytes.Equal(consent.GetAgentPeerId(), agentPeerID) ||
		!bytes.Equal(consent.GetNonce(), nonce) ||
		(consume && consent.GetExpiresAt() != expiresAt.Unix()) {
		return errors.New("spacelink consent does not match verified inputs")
	}

	// Preserve consumed markers written by older clients in this cache.
	// New consumption always goes to the account service.
	if err := a.checkCachedSpaceLinkNonce(ctx, agentPeerID, nonce, payload); err != nil {
		return err
	}

	// Resolve an existing signing Session; no cache-local fallback is safe.
	cli, _, _, err := a.getReadySessionClient(ctx)
	if err != nil {
		return err
	}
	return cli.accessSpaceLinkNonce(ctx, payload, consume)
}

// accessSpaceLinkNonce uses the account service's atomic consent reservation.
func (c *SessionClient) accessSpaceLinkNonce(ctx context.Context, payload []byte, consume bool) error {
	body, err := (&api.SpaceLinkNonceRequest{Payload: payload, Consume: consume}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "encode spacelink nonce request")
	}
	data, err := c.doPostBinary(ctx, "/api/account/spacelink/nonce", body, nil, SeedReasonMutation)
	if err != nil {
		return errors.Wrap(err, "access account spacelink nonce")
	}

	var response api.SpaceLinkNonceResponse
	if err := response.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "decode spacelink nonce response")
	}
	if response.GetConsumed() {
		return ErrSpaceLinkNonceConsumed
	}
	return nil
}

// checkCachedSpaceLinkNonce preserves pre-cloud consumption until its expiry.
// A missing local marker is never evidence of freshness.
func (a *ProviderAccount) checkCachedSpaceLinkNonce(ctx context.Context, agentPeerID, nonce, payload []byte) error {
	if a.objStore == nil {
		return nil
	}
	payloadDigest := sha256.Sum256(payload)
	key := []byte("spacelink-nonce/agent=" + hex.EncodeToString(agentPeerID) +
		"/nonce=" + hex.EncodeToString(nonce) + "/payload=" + hex.EncodeToString(payloadDigest[:]))

	return kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return a.objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			data, found, err := tx.Get(ctx, key)
			if err != nil || !found {
				return err
			}
			if len(data) != 8 {
				return errors.New("invalid cached spacelink nonce marker")
			}
			expiresAt := time.Unix(int64(binary.BigEndian.Uint64(data)), 0)
			if time.Now().Before(expiresAt) {
				return ErrSpaceLinkNonceConsumed
			}
			return nil
		},
	)
}

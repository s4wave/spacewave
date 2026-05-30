package provider_spacewave

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
)

func TestSpaceLinkNoncePreviewIsReadOnlyAndConsumeRejectsReplay(t *testing.T) {
	acc := &ProviderAccount{objStore: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
	ctx := context.Background()
	now := time.Unix(100, 0)
	expiresAt := now.Add(time.Minute)
	agentPeerID := []byte("agent-peer")
	nonce := []byte("nonce-1")
	payload := []byte("payload-1")

	if err := acc.checkSpaceLinkNonceFresh(ctx, agentPeerID, nonce, payload, now); err != nil {
		t.Fatalf("fresh check before consume: %v", err)
	}
	if err := acc.checkSpaceLinkNonceFresh(ctx, agentPeerID, nonce, payload, now); err != nil {
		t.Fatalf("fresh check repeated before consume: %v", err)
	}
	if err := acc.consumeSpaceLinkNonce(ctx, agentPeerID, nonce, payload, expiresAt, now); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if err := acc.checkSpaceLinkNonceFresh(ctx, agentPeerID, nonce, payload, now); !errors.Is(err, ErrSpaceLinkNonceConsumed) {
		t.Fatalf("fresh check after consume = %v, want ErrSpaceLinkNonceConsumed", err)
	}
	if err := acc.consumeSpaceLinkNonce(ctx, agentPeerID, nonce, payload, expiresAt, now); !errors.Is(err, ErrSpaceLinkNonceConsumed) {
		t.Fatalf("second consume = %v, want ErrSpaceLinkNonceConsumed", err)
	}
}

func TestSpaceLinkNonceKeyIncludesAgentNonceAndPayloadDigest(t *testing.T) {
	acc := &ProviderAccount{objStore: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
	ctx := context.Background()
	now := time.Unix(100, 0)
	expiresAt := now.Add(time.Minute)
	agentPeerID := []byte("agent-peer")
	nonce := []byte("nonce-1")
	payload := []byte("payload-1")

	if err := acc.consumeSpaceLinkNonce(ctx, agentPeerID, nonce, payload, expiresAt, now); err != nil {
		t.Fatalf("consume: %v", err)
	}
	cases := []struct {
		name        string
		agentPeerID []byte
		nonce       []byte
		payload     []byte
	}{
		{
			name:        "different agent",
			agentPeerID: []byte("agent-peer-2"),
			nonce:       nonce,
			payload:     payload,
		},
		{
			name:        "different nonce",
			agentPeerID: agentPeerID,
			nonce:       []byte("nonce-2"),
			payload:     payload,
		},
		{
			name:        "different payload",
			agentPeerID: agentPeerID,
			nonce:       nonce,
			payload:     []byte("payload-2"),
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := acc.checkSpaceLinkNonceFresh(ctx, tt.agentPeerID, tt.nonce, tt.payload, now); err != nil {
				t.Fatalf("fresh check: %v", err)
			}
		})
	}
}

func TestSpaceLinkNonceExpiredMarkerCanBeReusedLocally(t *testing.T) {
	acc := &ProviderAccount{objStore: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
	ctx := context.Background()
	now := time.Unix(100, 0)
	expiresAt := now.Add(time.Second)
	agentPeerID := []byte("agent-peer")
	nonce := []byte("nonce-1")
	payload := []byte("payload-1")

	if err := acc.consumeSpaceLinkNonce(ctx, agentPeerID, nonce, payload, expiresAt, now); err != nil {
		t.Fatalf("consume: %v", err)
	}
	afterMarkerExpiry := expiresAt.Add(spaceLinkNonceSkew).Add(time.Second)
	if err := acc.checkSpaceLinkNonceFresh(ctx, agentPeerID, nonce, payload, afterMarkerExpiry); err != nil {
		t.Fatalf("fresh check after marker expiry: %v", err)
	}
	if err := acc.consumeSpaceLinkNonce(ctx, agentPeerID, nonce, payload, afterMarkerExpiry.Add(time.Minute), afterMarkerExpiry); err != nil {
		t.Fatalf("consume after marker expiry: %v", err)
	}
}

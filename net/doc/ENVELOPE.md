# Envelope: Secret-Sharing Multi-Factor Authentication

The `envelope` package provides a sealed container whose encryption key is split
into Shamir secret shares distributed across grants. Each grant encrypts a
subset of shares to one or more keypairs, enabling multi-factor unlock policies.

## Concepts

**Envelope** -- An encrypted payload + a set of grants. The payload is encrypted
with XChaCha20-Poly1305 using a key derived (BLAKE3 KDF) from a Ristretto255
scalar. The scalar is split into Shamir shares via
[CIRCL](https://github.com/cloudflare/circl).

**Grant** -- An encrypted bundle of one or more Shamir shares. Each grant is
encrypted to one or more keypairs using `peer.EncryptToPubKey`. Decrypting any
one of the grant's keypairs yields the shares inside.

**Threshold** -- The CIRCL threshold parameter `t`. Recovery requires `t + 1`
shares. With `t = 0`, any single share suffices (the polynomial is degree 0).

**Context** -- A string that binds the envelope to an application-specific
purpose. Must be identical at seal and unseal time. Stored as a BLAKE3 hash in
the envelope for verification.

## Threshold Semantics

| Threshold | Shares Needed | Use Case                                |
| --------- | ------------- | --------------------------------------- |
| 0         | 1             | Any single grant can unlock (OR policy) |
| 1         | 2             | Two-factor: need two different grants   |
| N         | N + 1         | (N+1)-of-M multi-factor                 |

With threshold=0, CIRCL generates a degree-0 polynomial where every share
evaluates to the same secret. This is the correct mode for "any one key
suffices" policies (e.g., local session storage). For multi-factor
authentication, set threshold >= 1.

## CLI Usage

### Seal

```bash
# Seal stdin to a single key (threshold=0, any key unlocks)
echo "secret data" | bifrost envelope seal -k key.pem -o sealed.bin

# Seal a file to two keys with threshold=1 (need both keys)
bifrost envelope seal -k key1.pem -k key2.pem -t 1 -i secret.txt -o sealed.bin

# Seal with a custom context string
bifrost envelope seal -k key.pem --ctx "myapp/session v1" -i data.bin -o env.bin
```

### Unseal

```bash
# Unseal with a single key
bifrost envelope unseal -k key.pem -i sealed.bin > plaintext.txt

# Unseal with multiple keys (for threshold > 0)
bifrost envelope unseal -k key1.pem -k key2.pem -i sealed.bin -o plaintext.txt

# Show envelope info without decrypting payload
bifrost envelope unseal -k key.pem -i sealed.bin --info
```

### Info Output

The `--info` flag outputs JSON metadata about the envelope:

```json
{
  "success": true,
  "shares_available": 2,
  "shares_needed": 2,
  "unlocked_grants": [0, 1],
  "total_grants": 2,
  "total_keypairs": 2,
  "threshold": 1,
  "envelope_id": "834145bab7198bb2e8408e703d04fe9e"
}
```

## Go API

### Build (Seal)

```go
import (
    "crypto/rand"
    "github.com/s4wave/spacewave/net/envelope"
    "github.com/s4wave/spacewave/net/crypto"
)

env, err := envelope.BuildEnvelope(
    rand.Reader,
    "myapp/data v1",                 // context string
    payload,                          // plaintext bytes
    []crypto.PubKey{pubKey1, pubKey2}, // recipient public keys
    &envelope.EnvelopeConfig{
        Threshold: 1,                 // need 2 shares
        GrantConfigs: []*envelope.EnvelopeGrantConfig{
            {ShareCount: 1, KeypairIndexes: []uint32{0}}, // 1 share to key 0
            {ShareCount: 1, KeypairIndexes: []uint32{1}}, // 1 share to key 1
        },
    },
)
```

### Unlock (Unseal)

```go
payload, result, err := envelope.UnlockEnvelope(
    "myapp/data v1",                    // must match seal context
    env,                                 // the sealed Envelope
    []crypto.PrivKey{privKey1, privKey2}, // private keys to try
)

if err != nil {
    // Invalid envelope, context mismatch, etc.
}
if !result.GetSuccess() {
    // Not enough shares. Check result.SharesAvailable vs result.SharesNeeded.
}
// payload contains the decrypted data
```

### Grant Configurations

A grant can encrypt shares to multiple keypairs (any one can decrypt):

```go
// One grant, encrypted to both keys (either key can decrypt this grant)
&envelope.EnvelopeGrantConfig{
    ShareCount:     1,
    KeypairIndexes: []uint32{0, 1}, // encrypted to both key 0 and key 1
}
```

This is useful for "OR within a factor" -- e.g., either a hardware key or a
recovery key can provide the same share.

## Cryptographic Details

1. **Secret Generation**: Random non-zero Ristretto255 scalar via CIRCL
2. **Key Derivation**: BLAKE3 `DeriveKey` from scalar bytes with length-prefixed context
3. **Payload Encryption**: XChaCha20-Poly1305 with random 24-byte nonce
4. **Secret Splitting**: CIRCL Shamir secret sharing over Ristretto255 scalar field
5. **Grant Encryption**: `peer.EncryptToPubKey` (Ed25519 ECDH + ChaCha20-Poly1305)
6. **Public Key Format**: PEM-encoded via `bifrost/keypem`
7. **Envelope ID**: BLAKE3 hash of (secret || context), hex-encoded, 16 bytes

## Proto Definition

See `envelope/envelope.proto` for the complete wire format.

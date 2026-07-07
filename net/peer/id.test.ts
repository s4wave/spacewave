import { describe, expect, it } from 'vitest'

import { base58Encode } from './base58.js'
import {
  extractEd25519PubkeyFromPeerID,
  extractPublicKeyFromPeerID,
  KEY_TYPE_ED25519,
  KEY_TYPE_RSA,
  parsePublicKeyFromProtobuf,
  verifyKeypairPubkey,
} from './id.js'

function hexToBytes(hex: string): Uint8Array {
  const bytes = new Uint8Array(hex.length / 2)
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16)
  }
  return bytes
}

function encodeVarint(value: number): Uint8Array {
  const bytes: number[] = []
  while (value > 0x7f) {
    bytes.push((value & 0x7f) | 0x80)
    value >>>= 7
  }
  bytes.push(value & 0x7f)
  return new Uint8Array(bytes)
}

function encodePublicKeyProto(
  keyType: number,
  keyData: Uint8Array,
): Uint8Array {
  const typeBytes = encodeVarint(keyType)
  const dataLen = encodeVarint(keyData.length)
  const result = new Uint8Array(
    1 + typeBytes.length + 1 + dataLen.length + keyData.length,
  )
  let offset = 0
  result[offset++] = 0x08
  result.set(typeBytes, offset)
  offset += typeBytes.length
  result[offset++] = 0x12
  result.set(dataLen, offset)
  offset += dataLen.length
  result.set(keyData, offset)
  return result
}

function encodePeerID(publicKeyProto: Uint8Array): string {
  const digestLen = encodeVarint(publicKeyProto.length)
  const multihash = new Uint8Array(1 + digestLen.length + publicKeyProto.length)
  multihash[0] = 0x00
  multihash.set(digestLen, 1)
  multihash.set(publicKeyProto, 1 + digestLen.length)
  return base58Encode(multihash)
}

function encodePeerIDWithMultihashCode(
  code: number,
  publicKeyProto: Uint8Array,
): string {
  const codeBytes = encodeVarint(code)
  const digestLen = encodeVarint(publicKeyProto.length)
  const multihash = new Uint8Array(
    codeBytes.length + digestLen.length + publicKeyProto.length,
  )
  let offset = 0
  multihash.set(codeBytes, offset)
  offset += codeBytes.length
  multihash.set(digestLen, offset)
  offset += digestLen.length
  multihash.set(publicKeyProto, offset)
  return base58Encode(multihash)
}

describe('parsePublicKeyFromProtobuf', () => {
  it('returns key type and bytes for libp2p PublicKey protobuf data', () => {
    const cases = [
      {
        name: 'ed25519',
        keyType: KEY_TYPE_ED25519,
        keyData: new Uint8Array(32).map((_, i) => i),
      },
      {
        name: 'rsa',
        keyType: KEY_TYPE_RSA,
        keyData: new Uint8Array([1, 2, 3, 4, 5, 6]),
      },
    ]

    for (const testCase of cases) {
      const proto = encodePublicKeyProto(testCase.keyType, testCase.keyData)

      expect(parsePublicKeyFromProtobuf(proto), testCase.name).toEqual({
        keyType: testCase.keyType,
        keyData: testCase.keyData,
      })
    }
  })
})

describe('extractPublicKeyFromPeerID', () => {
  it('extracts key type and bytes from an identity multihash peer id', () => {
    const keyData = new Uint8Array(32).map((_, i) => i + 10)
    const proto = encodePublicKeyProto(KEY_TYPE_ED25519, keyData)
    const peerID = encodePeerID(proto)

    expect(extractPublicKeyFromPeerID(peerID)).toEqual({
      keyType: KEY_TYPE_ED25519,
      keyData,
    })
  })

  it('rejects peer ids whose multihash code is not identity', () => {
    const keyData = new Uint8Array([7, 8, 9])
    const proto = encodePublicKeyProto(KEY_TYPE_RSA, keyData)

    expect(
      extractPublicKeyFromPeerID(encodePeerIDWithMultihashCode(0x12, proto)),
    ).toBeNull()
  })
})

describe('extractEd25519PubkeyFromPeerID', () => {
  it('returns raw Ed25519 public keys from identity peer ids', () => {
    const keyData = new Uint8Array(32).map((_, i) => 255 - i)
    const proto = encodePublicKeyProto(KEY_TYPE_ED25519, keyData)

    expect(extractEd25519PubkeyFromPeerID(encodePeerID(proto))).toEqual(keyData)
  })

  it('rejects decoded keys that are not 32-byte Ed25519 public keys', () => {
    const rsaKeyData = new Uint8Array([1, 2, 3, 4, 5, 6])
    const shortEd25519KeyData = new Uint8Array(31).fill(4)

    expect(
      extractEd25519PubkeyFromPeerID(
        encodePeerID(encodePublicKeyProto(KEY_TYPE_RSA, rsaKeyData)),
      ),
    ).toBeNull()
    expect(
      extractEd25519PubkeyFromPeerID(
        encodePeerID(
          encodePublicKeyProto(KEY_TYPE_ED25519, shortEd25519KeyData),
        ),
      ),
    ).toBeNull()
  })
})

describe('verifyKeypairPubkey', () => {
  it('accepts public keys when their bytes match the peer id protobuf data', () => {
    const keyData = new Uint8Array([1, 2, 3, 4, 5, 6])
    const peerID = encodePeerID(encodePublicKeyProto(KEY_TYPE_RSA, keyData))

    expect(verifyKeypairPubkey(peerID, keyData)).toBeNull()
  })

  it('rejects public keys whose bytes differ from the peer id protobuf data', () => {
    const peerID = encodePeerID(
      encodePublicKeyProto(KEY_TYPE_ED25519, new Uint8Array(32).fill(7)),
    )

    expect(verifyKeypairPubkey(peerID, new Uint8Array(32).fill(8))).toBe(
      'pubkey does not match peer_id',
    )
  })
})

describe('bifrost peer id vector', () => {
  it('matches the Go encoder for a fixed Ed25519 public key', () => {
    const rawKey = hexToBytes(
      '000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f',
    )
    const proto = hexToBytes(
      '08011220000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f',
    )
    const peerID = '12D3KooW9pP4Seg3kZYhySpuVjn1RPdQBsUFZKiFxGMGQN5MeL6A'

    expect(parsePublicKeyFromProtobuf(proto)).toEqual({
      keyType: KEY_TYPE_ED25519,
      keyData: rawKey,
    })
    expect(extractPublicKeyFromPeerID(peerID)).toEqual({
      keyType: KEY_TYPE_ED25519,
      keyData: rawKey,
    })
  })
})

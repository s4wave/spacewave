import { PublicKey, KeyType } from '../crypto/crypto.pb.js'

import { base58Decode } from './base58.js'

const MULTIHASH_IDENTITY_CODE = 0

// KEY_TYPE_RSA is the libp2p RSA public key type.
export const KEY_TYPE_RSA = KeyType.RSA

// KEY_TYPE_ED25519 is the libp2p Ed25519 public key type.
export const KEY_TYPE_ED25519 = KeyType.Ed25519

// DecodedPeerPublicKey is a public key extracted from a peer ID.
export interface DecodedPeerPublicKey {
  keyType: KeyType
  keyData: Uint8Array
}

// extractPublicKeyFromPeerID extracts the embedded protobuf public key from a
// base58-encoded peer ID.
export function extractPublicKeyFromPeerID(
  peerID: string,
): DecodedPeerPublicKey | null {
  const multihash = base58Decode(peerID)
  if (!multihash || multihash.length < 2) {
    return null
  }

  const code = readUvarint(multihash, 0)
  if (!code || code.value !== MULTIHASH_IDENTITY_CODE) {
    return null
  }

  const digestLen = readUvarint(multihash, code.nextOffset)
  if (!digestLen) {
    return null
  }
  const digestStart = digestLen.nextOffset
  const digestEnd = digestStart + digestLen.value
  if (digestEnd !== multihash.length) {
    return null
  }

  return parsePublicKeyFromProtobuf(multihash.subarray(digestStart, digestEnd))
}

// parsePublicKeyFromProtobuf extracts key type and key bytes from a libp2p
// PublicKey protobuf message.
export function parsePublicKeyFromProtobuf(
  data: Uint8Array,
): DecodedPeerPublicKey | null {
  let publicKey: PublicKey
  try {
    publicKey = PublicKey.fromBinary(data)
  } catch {
    return null
  }
  if (publicKey.keyType === undefined || !publicKey.data) {
    return null
  }

  return { keyType: publicKey.keyType, keyData: publicKey.data }
}

// parseEd25519FromProtobuf extracts a 32-byte Ed25519 public key from a libp2p
// PublicKey protobuf message.
export function parseEd25519FromProtobuf(data: Uint8Array): Uint8Array | null {
  const decoded = parsePublicKeyFromProtobuf(data)
  if (!decoded) {
    return null
  }
  if (decoded.keyType !== KEY_TYPE_ED25519 || decoded.keyData.length !== 32) {
    return null
  }
  return decoded.keyData
}

// extractEd25519PubkeyFromPeerID extracts the raw Ed25519 public key from a
// base58-encoded peer ID.
export function extractEd25519PubkeyFromPeerID(
  peerID: string,
): Uint8Array | null {
  const decoded = extractPublicKeyFromPeerID(peerID)
  if (!decoded) {
    return null
  }
  if (decoded.keyType !== KEY_TYPE_ED25519 || decoded.keyData.length !== 32) {
    return null
  }
  return decoded.keyData
}

// verifyKeypairPubkey checks that a peer ID's embedded public key matches the
// provided raw pubkey bytes. Returns null on match, or an error string.
export function verifyKeypairPubkey(
  peerID: string,
  pubkey: Uint8Array,
): string | null {
  if (!peerID) {
    return 'missing peer_id'
  }
  if (pubkey.length === 0) {
    return 'missing pubkey'
  }
  const derived = extractPublicKeyFromPeerID(peerID)
  if (!derived) {
    return 'cannot extract pubkey from peer_id'
  }
  if (
    derived.keyData.length !== pubkey.length ||
    !derived.keyData.every((b, i) => b === pubkey[i])
  ) {
    return 'pubkey does not match peer_id'
  }
  return null
}

interface ReadUvarintResult {
  value: number
  nextOffset: number
}

function readUvarint(
  data: Uint8Array,
  offset: number,
): ReadUvarintResult | null {
  let value = 0
  let shift = 0
  let nextOffset = offset

  while (nextOffset < data.length) {
    const byte = data[nextOffset]
    value |= (byte & 0x7f) << shift
    nextOffset++
    if ((byte & 0x80) === 0) {
      return { value, nextOffset }
    }
    shift += 7
  }

  return null
}

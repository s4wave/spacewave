import { blake3 } from '@noble/hashes/blake3.js'
import { sha256 } from '@noble/hashes/sha2.js'

import { HashType } from '../hash/hash.pb.js'
import {
  KEY_TYPE_ED25519,
  parsePublicKeyFromProtobuf,
  peerIDFromPublicKey,
} from './id.js'
import type { Signature } from './peer.pb.js'

// SIGN_SEP separates the fields of a signed body.
const SIGN_SEP = ' - SIGN - '

// verifySignature checks a peer signature over data under encContext and
// returns the signer's peer ID, or null when the signature is invalid.
// The signature must carry its public key; only Ed25519 keys are supported.
// The signed body matches Go peer.NewSignature: the context, the hash type and
// the digest of data, joined by SIGN_SEP.
export async function verifySignature(
  encContext: string,
  signature: Signature | undefined,
  data: Uint8Array,
): Promise<string | null> {
  // Recover the signer from the included public key.
  const pubKey = signature?.pubKey
  const decoded = pubKey ? parsePublicKeyFromProtobuf(pubKey) : null
  if (
    !pubKey ||
    !decoded ||
    decoded.keyType !== KEY_TYPE_ED25519 ||
    decoded.keyData.length !== 32
  ) {
    return null
  }
  const signer = peerIDFromPublicKey(pubKey)
  if (!signer) {
    return null
  }

  // Digest the data with the signature's hash type.
  const hashType = signature?.hashType
  let digest: Uint8Array
  if (hashType === HashType.HashType_SHA256) {
    digest = sha256(data)
  } else if (hashType === HashType.HashType_BLAKE3) {
    digest = blake3(data)
  } else {
    return null
  }

  // Verify Ed25519 over the signed body.
  const encoder = new TextEncoder()
  const prefix = encoder.encode(
    `${encContext}${SIGN_SEP}${hashType}${SIGN_SEP}`,
  )
  const body = new Uint8Array(prefix.length + digest.length)
  body.set(prefix)
  body.set(digest, prefix.length)
  const key = await crypto.subtle.importKey(
    'raw',
    new Uint8Array(decoded.keyData),
    { name: 'Ed25519' },
    false,
    ['verify'],
  )
  const valid = await crypto.subtle.verify(
    'Ed25519',
    key,
    new Uint8Array(signature?.sigData ?? []),
    body,
  )
  return valid ? signer : null
}

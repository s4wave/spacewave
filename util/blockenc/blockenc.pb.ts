/* eslint-disable */

export const protobufPackage = 'blockenc'

/**
 * BlockEnc is the block encryption method to use.
 * Most methods use 32 byte keys.
 */
export enum BlockEnc {
  /** BlockEnc_UNKNOWN - BlockEnc_UNKNOWN defaults to BlockEnc_XCHACHA20_POLY1305. */
  BlockEnc_UNKNOWN = 0,
  /** BlockEnc_NONE - BlockEnc_NONE is unencrypted. */
  BlockEnc_NONE = 1,
  /**
   * BlockEnc_XCHACHA20_POLY1305 - BlockEnc_XCHACHA20_POLY1305 uses extended chacha encryption.
   * Key size of 32 bytes.
   * Derives the nonce with blake3 key derivation.
   * Stores the nonce in the first 24 bytes of the ciphertext.
   */
  BlockEnc_XCHACHA20_POLY1305 = 2,
  /**
   * BlockEnc_SECRET_BOX - BlockCrypt_SECRET_BOX uses nacl secret box encryption.
   * Derives the nonce with blake3 key derivation.
   * Stores the nonce in the first 24 bytes of the ciphertext.
   */
  BlockEnc_SECRET_BOX = 3,
  UNRECOGNIZED = -1,
}

export function blockEncFromJSON(object: any): BlockEnc {
  switch (object) {
    case 0:
    case 'BlockEnc_UNKNOWN':
      return BlockEnc.BlockEnc_UNKNOWN
    case 1:
    case 'BlockEnc_NONE':
      return BlockEnc.BlockEnc_NONE
    case 2:
    case 'BlockEnc_XCHACHA20_POLY1305':
      return BlockEnc.BlockEnc_XCHACHA20_POLY1305
    case 3:
    case 'BlockEnc_SECRET_BOX':
      return BlockEnc.BlockEnc_SECRET_BOX
    case -1:
    case 'UNRECOGNIZED':
    default:
      return BlockEnc.UNRECOGNIZED
  }
}

export function blockEncToJSON(object: BlockEnc): string {
  switch (object) {
    case BlockEnc.BlockEnc_UNKNOWN:
      return 'BlockEnc_UNKNOWN'
    case BlockEnc.BlockEnc_NONE:
      return 'BlockEnc_NONE'
    case BlockEnc.BlockEnc_XCHACHA20_POLY1305:
      return 'BlockEnc_XCHACHA20_POLY1305'
    case BlockEnc.BlockEnc_SECRET_BOX:
      return 'BlockEnc_SECRET_BOX'
    case BlockEnc.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

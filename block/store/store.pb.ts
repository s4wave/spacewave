/* eslint-disable */

export const protobufPackage = 'block.store'

/**
 * BlockStoreMode controls the mode for the block store overlay.
 * Overlays an upper block store over a lower store.
 */
export enum BlockStoreMode {
  /**
   * BlockStoreMode_DIRECT - BlockStoreMode_DIRECT is the direct block store mode.
   * reads and writes go to the upper store bypassing the lower block store.
   */
  BlockStoreMode_DIRECT = 0,
  /**
   * BlockStoreMode_CACHE - BlockStoreMode_CACHE uses the upper store as a cache for the lower store.
   * reads go to the upper store, then the lower store.
   * writes go to the lower store only
   * reads that miss the upper store are written back to the upper store
   */
  BlockStoreMode_CACHE = 1,
  /**
   * BlockStoreMode_CACHE_LOWER - BlockStoreMode_CACHE_LOWER uses the lower store as a cache for the upper store.
   * reads go to the lower store, then the upper store.
   * writes go to the upper store only
   * reads that miss the lower store are written back to the lower store
   */
  BlockStoreMode_CACHE_LOWER = 2,
  UNRECOGNIZED = -1,
}

export function blockStoreModeFromJSON(object: any): BlockStoreMode {
  switch (object) {
    case 0:
    case 'BlockStoreMode_DIRECT':
      return BlockStoreMode.BlockStoreMode_DIRECT
    case 1:
    case 'BlockStoreMode_CACHE':
      return BlockStoreMode.BlockStoreMode_CACHE
    case 2:
    case 'BlockStoreMode_CACHE_LOWER':
      return BlockStoreMode.BlockStoreMode_CACHE_LOWER
    case -1:
    case 'UNRECOGNIZED':
    default:
      return BlockStoreMode.UNRECOGNIZED
  }
}

export function blockStoreModeToJSON(object: BlockStoreMode): string {
  switch (object) {
    case BlockStoreMode.BlockStoreMode_DIRECT:
      return 'BlockStoreMode_DIRECT'
    case BlockStoreMode.BlockStoreMode_CACHE:
      return 'BlockStoreMode_CACHE'
    case BlockStoreMode.BlockStoreMode_CACHE_LOWER:
      return 'BlockStoreMode_CACHE_LOWER'
    case BlockStoreMode.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

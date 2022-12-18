/* eslint-disable */

export const protobufPackage = 'unixfs.sync'

/** DeleteMode is the set of available delete modes for Sync. */
export enum DeleteMode {
  /** DeleteMode_NONE - DeleteMode_NONE does not delete files from the destination. */
  DeleteMode_NONE = 0,
  /**
   * DeleteMode_BEFORE - DeleteMode_BEFORE scans & deletes files from the destination before writing
   * any new files or directories to the destination.
   */
  DeleteMode_BEFORE = 1,
  /** DeleteMode_DURING - DeleteMode_DURING scans & deletes files from the destination while writing. */
  DeleteMode_DURING = 2,
  /** DeleteMode_AFTER - DeleteMode_AFTER scans & deletes files from the destination after writing. */
  DeleteMode_AFTER = 3,
  /** DeleteMode_ONLY - DeleteMode_ONLY scans & deletes files and skips writing any new data. */
  DeleteMode_ONLY = 4,
  UNRECOGNIZED = -1,
}

export function deleteModeFromJSON(object: any): DeleteMode {
  switch (object) {
    case 0:
    case 'DeleteMode_NONE':
      return DeleteMode.DeleteMode_NONE
    case 1:
    case 'DeleteMode_BEFORE':
      return DeleteMode.DeleteMode_BEFORE
    case 2:
    case 'DeleteMode_DURING':
      return DeleteMode.DeleteMode_DURING
    case 3:
    case 'DeleteMode_AFTER':
      return DeleteMode.DeleteMode_AFTER
    case 4:
    case 'DeleteMode_ONLY':
      return DeleteMode.DeleteMode_ONLY
    case -1:
    case 'UNRECOGNIZED':
    default:
      return DeleteMode.UNRECOGNIZED
  }
}

export function deleteModeToJSON(object: DeleteMode): string {
  switch (object) {
    case DeleteMode.DeleteMode_NONE:
      return 'DeleteMode_NONE'
    case DeleteMode.DeleteMode_BEFORE:
      return 'DeleteMode_BEFORE'
    case DeleteMode.DeleteMode_DURING:
      return 'DeleteMode_DURING'
    case DeleteMode.DeleteMode_AFTER:
      return 'DeleteMode_AFTER'
    case DeleteMode.DeleteMode_ONLY:
      return 'DeleteMode_ONLY'
    case DeleteMode.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

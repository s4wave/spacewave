import type { FSCursorDirent, FSCursorNodeType } from './fs-cursor.js'
import type { FileInfo } from './file-info.js'
import { nodeTypeToMode } from './fs-node-type.js'

// ModeDir is the directory bit in a file mode (matching Go os.ModeDir).
const ModeDir = 0x80000000
// ModeIrregular is the irregular bit in a file mode.
const ModeIrregular = 0x00080000
// ModeTypeMask covers all type bits.
const ModeTypeMask =
  ModeDir |
  0x08000000 |
  ModeIrregular |
  0x04000000 |
  0x02000000 |
  0x01000000 |
  0x00800000 |
  0x00400000 |
  0x00200000 |
  0x00100000

// FSDirEntry implements a directory entry with a FSCursorDirent and associated FileInfo.
export class FSDirEntry {
  private readonly ent: FSCursorDirent
  private readonly fileInfo: FileInfo | null

  constructor(ent: FSCursorDirent, fileInfo: FileInfo | null) {
    this.ent = ent
    this.fileInfo = fileInfo
  }

  // getName returns the name of the file or subdirectory described by the entry.
  // This name is only the final element of the path (the base name).
  getName(): string {
    return this.ent.getName()
  }

  // isDir reports whether the entry describes a directory.
  isDir(): boolean {
    return this.ent.getIsDirectory()
  }

  // getType returns the type bits for the entry.
  getType(): number {
    let defaultMode: number
    if (this.fileInfo) {
      defaultMode = this.fileInfo.getMode()
    } else if (this.ent.getIsDirectory()) {
      defaultMode = ModeDir | 0o555
    } else {
      defaultMode = 0o444
    }

    const typ = nodeTypeToMode(this.ent, defaultMode & 0o777)
    if (typ === ModeIrregular) {
      return defaultMode & ModeTypeMask
    }
    return typ & ModeTypeMask
  }

  // getInfo returns the FileInfo for the file or subdirectory.
  // Throws if file info is unavailable.
  getInfo(): FileInfo {
    if (!this.fileInfo) {
      throw new Error('file info unavailable')
    }
    return this.fileInfo
  }
}

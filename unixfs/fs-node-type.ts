import type { FSCursorNodeType } from './fs-cursor.js'

// ModeDir is the directory bit in a file mode (matching Go os.ModeDir).
const ModeDir = 0x80000000
// ModeSymlink is the symlink bit in a file mode (matching Go os.ModeSymlink).
const ModeSymlink = 0x08000000
// ModeIrregular is the irregular bit in a file mode (matching Go os.ModeIrregular).
const ModeIrregular = 0x00080000
// ModePerm is the permission bits mask (matching Go fs.ModePerm).
const ModePerm = 0o777
// ModeTypeMask is the type bits mask (matching Go fs.ModeType).
const ModeTypeMask =
  ModeDir |
  ModeSymlink |
  ModeIrregular |
  0x04000000 |
  0x02000000 |
  0x01000000 |
  0x00800000 |
  0x00400000 |
  0x00200000 |
  0x00100000

// fsCursorNodeType is a static node type value.
class StaticFSCursorNodeType implements FSCursorNodeType {
  private isDir: boolean
  private isFile: boolean
  private isSymlinkVal: boolean

  constructor(isDir: boolean, isFile: boolean, isSymlink: boolean) {
    this.isDir = isDir
    this.isFile = isFile
    this.isSymlinkVal = isSymlink
  }

  getIsDirectory(): boolean {
    return this.isDir
  }

  getIsFile(): boolean {
    return this.isFile
  }

  getIsSymlink(): boolean {
    return this.isSymlinkVal
  }
}

// newFSCursorNodeType_Unknown constructs a FSCursorNodeType with no type.
export function newFSCursorNodeType_Unknown(): FSCursorNodeType {
  return new StaticFSCursorNodeType(false, false, false)
}

// newFSCursorNodeType_File constructs a FSCursorNodeType for a file.
export function newFSCursorNodeType_File(): FSCursorNodeType {
  return new StaticFSCursorNodeType(false, true, false)
}

// newFSCursorNodeType_Dir constructs a FSCursorNodeType for a directory.
export function newFSCursorNodeType_Dir(): FSCursorNodeType {
  return new StaticFSCursorNodeType(true, false, false)
}

// newFSCursorNodeType_Symlink constructs a FSCursorNodeType for a symlink.
export function newFSCursorNodeType_Symlink(): FSCursorNodeType {
  return new StaticFSCursorNodeType(false, false, true)
}

// defaultPermissions returns the default permissions set for a filetype.
export function defaultPermissions(nt: FSCursorNodeType): number {
  if (nt.getIsSymlink()) {
    return 0o777
  }
  if (nt.getIsDirectory()) {
    return 0o755
  }
  return 0o644
}

// nodeTypeToMode converts a FSCursorNodeType into a mode value.
export function nodeTypeToMode(
  nodeType: FSCursorNodeType,
  permissions: number,
): number {
  permissions = permissions & ModePerm
  if (nodeType.getIsSymlink()) {
    return permissions | ModeSymlink
  }
  if (nodeType.getIsDirectory()) {
    return permissions | ModeDir
  }
  if (nodeType.getIsFile()) {
    return permissions
  }
  return ModeIrregular
}

// fileModeToNodeType converts a file mode to a FSCursorNodeType.
// Throws if the mode represents an unsupported type.
export function fileModeToNodeType(mode: number): FSCursorNodeType {
  if ((mode & ModeDir) !== 0) {
    return newFSCursorNodeType_Dir()
  }
  if ((mode & ModeTypeMask) === 0) {
    return newFSCursorNodeType_File()
  }
  if ((mode & ModeSymlink) !== 0) {
    return newFSCursorNodeType_Symlink()
  }
  throw new Error('unsupported mode / node type: 0x' + mode.toString(16))
}

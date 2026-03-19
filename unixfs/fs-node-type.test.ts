import { describe, it, expect } from 'vitest'
import {
  defaultPermissions,
  nodeTypeToMode,
  fileModeToNodeType,
  newFSCursorNodeType_Dir,
  newFSCursorNodeType_File,
  newFSCursorNodeType_Symlink,
  newFSCursorNodeType_Unknown,
} from './fs-node-type.js'

// Mode constants matching the source.
const ModeDir = 0x80000000
const ModeSymlink = 0x08000000
const ModeIrregular = 0x00080000
const ModePerm = 0o777

describe('defaultPermissions', () => {
  it('returns 0o755 for directory', () => {
    expect(defaultPermissions(newFSCursorNodeType_Dir())).toBe(0o755)
  })

  it('returns 0o644 for file', () => {
    expect(defaultPermissions(newFSCursorNodeType_File())).toBe(0o644)
  })

  it('returns 0o777 for symlink', () => {
    expect(defaultPermissions(newFSCursorNodeType_Symlink())).toBe(0o777)
  })

  it('returns 0o644 for unknown type (falls through to file default)', () => {
    expect(defaultPermissions(newFSCursorNodeType_Unknown())).toBe(0o644)
  })
})

describe('nodeTypeToMode', () => {
  it('returns directory mode', () => {
    const mode = nodeTypeToMode(newFSCursorNodeType_Dir(), 0o755)
    // JS bitwise ops use signed 32-bit, so compare with >>> 0 for unsigned.
    expect((mode & ModeDir) >>> 0).toBe(ModeDir >>> 0)
    expect(mode & ModePerm).toBe(0o755)
  })

  it('returns file mode without type bits', () => {
    const mode = nodeTypeToMode(newFSCursorNodeType_File(), 0o644)
    expect(mode & ModeDir).toBe(0)
    expect(mode & ModeSymlink).toBe(0)
    expect(mode & ModePerm).toBe(0o644)
  })

  it('returns symlink mode', () => {
    const mode = nodeTypeToMode(newFSCursorNodeType_Symlink(), 0o777)
    expect(mode & ModeSymlink).toBe(ModeSymlink)
    expect(mode & ModePerm).toBe(0o777)
  })

  it('returns ModeIrregular for unknown type', () => {
    const mode = nodeTypeToMode(newFSCursorNodeType_Unknown(), 0o644)
    expect(mode).toBe(ModeIrregular)
  })

  it('masks permissions to only perm bits', () => {
    const mode = nodeTypeToMode(newFSCursorNodeType_File(), 0xffffffff)
    expect(mode & ModePerm).toBe(ModePerm)
    expect(mode & ~ModePerm).toBe(0)
  })
})

describe('fileModeToNodeType', () => {
  it('converts directory mode to dir node type', () => {
    const nt = fileModeToNodeType(ModeDir | 0o755)
    expect(nt.getIsDirectory()).toBe(true)
    expect(nt.getIsFile()).toBe(false)
    expect(nt.getIsSymlink()).toBe(false)
  })

  it('converts regular file mode to file node type', () => {
    const nt = fileModeToNodeType(0o644)
    expect(nt.getIsFile()).toBe(true)
    expect(nt.getIsDirectory()).toBe(false)
    expect(nt.getIsSymlink()).toBe(false)
  })

  it('converts symlink mode to symlink node type', () => {
    const nt = fileModeToNodeType(ModeSymlink | 0o777)
    expect(nt.getIsSymlink()).toBe(true)
    expect(nt.getIsFile()).toBe(false)
    expect(nt.getIsDirectory()).toBe(false)
  })

  it('throws for unsupported mode', () => {
    expect(() => fileModeToNodeType(ModeIrregular)).toThrow(
      'unsupported mode / node type',
    )
  })
})

describe('nodeTypeToMode / fileModeToNodeType round-trip', () => {
  it('round-trips directory', () => {
    const dir = newFSCursorNodeType_Dir()
    const mode = nodeTypeToMode(dir, 0o755)
    const restored = fileModeToNodeType(mode)
    expect(restored.getIsDirectory()).toBe(true)
    expect(restored.getIsFile()).toBe(false)
    expect(restored.getIsSymlink()).toBe(false)
  })

  it('round-trips file', () => {
    const file = newFSCursorNodeType_File()
    const mode = nodeTypeToMode(file, 0o644)
    const restored = fileModeToNodeType(mode)
    expect(restored.getIsFile()).toBe(true)
    expect(restored.getIsDirectory()).toBe(false)
    expect(restored.getIsSymlink()).toBe(false)
  })

  it('round-trips symlink', () => {
    const sym = newFSCursorNodeType_Symlink()
    const mode = nodeTypeToMode(sym, 0o777)
    const restored = fileModeToNodeType(mode)
    expect(restored.getIsSymlink()).toBe(true)
    expect(restored.getIsFile()).toBe(false)
    expect(restored.getIsDirectory()).toBe(false)
  })
})

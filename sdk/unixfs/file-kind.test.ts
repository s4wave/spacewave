import { describe, expect, it } from 'vitest'
import {
  getUnixFSDirEntryKind,
  getUnixFSFileInfoKind,
  getUnixFSNodeTypeKind,
  UnixFSModeSymlink,
} from './file-kind.js'

describe('UnixFS file kind', () => {
  it('projects file info into file, directory, symlink, and unknown kinds', () => {
    expect(getUnixFSFileInfoKind({ isDir: false })).toBe('file')
    expect(getUnixFSFileInfoKind({ isDir: true })).toBe('directory')
    expect(
      getUnixFSFileInfoKind({ isDir: false, mode: UnixFSModeSymlink }),
    ).toBe('symlink')
    expect(getUnixFSFileInfoKind({})).toBe('unknown')
  })

  it('projects directory entries without changing generic row shape', () => {
    expect(getUnixFSDirEntryKind({ isDir: false })).toBe('file')
    expect(getUnixFSDirEntryKind({ isDir: true })).toBe('directory')
    expect(getUnixFSDirEntryKind({ isDir: false, isSymlink: true })).toBe(
      'symlink',
    )
    expect(getUnixFSDirEntryKind({})).toBe('unknown')
  })

  it('projects node type responses with symlink taking precedence', () => {
    expect(getUnixFSNodeTypeKind({ isFile: true })).toBe('file')
    expect(getUnixFSNodeTypeKind({ isDir: true })).toBe('directory')
    expect(getUnixFSNodeTypeKind({ isFile: true, isSymlink: true })).toBe(
      'symlink',
    )
    expect(getUnixFSNodeTypeKind({})).toBe('unknown')
  })
})

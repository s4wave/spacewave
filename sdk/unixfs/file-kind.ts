import type { DirEntry, FileInfo, NodeType } from './handle.pb.js'

export type UnixFSFileKind = 'file' | 'directory' | 'symlink' | 'unknown'

export const UnixFSModeSymlink = 0x08000000

export function getUnixFSFileInfoKind(
  info: FileInfo | null | undefined,
): UnixFSFileKind {
  if (!info) {
    return 'unknown'
  }
  if (((info.mode ?? 0) & UnixFSModeSymlink) !== 0) {
    return 'symlink'
  }
  // Decoded proto3 bools omit false values, so an unset isDir is a file.
  return info.isDir ? 'directory' : 'file'
}

export function getUnixFSDirEntryKind(
  entry: DirEntry | null | undefined,
): UnixFSFileKind {
  if (!entry) {
    return 'unknown'
  }
  if (entry.isSymlink === true) {
    return 'symlink'
  }
  // Decoded proto3 bools omit false values, so an unset isDir is a file.
  return entry.isDir ? 'directory' : 'file'
}

export function getUnixFSNodeTypeKind(
  nodeType: NodeType | null | undefined,
): UnixFSFileKind {
  if (!nodeType) {
    return 'unknown'
  }
  if (nodeType.isSymlink === true) {
    return 'symlink'
  }
  if (nodeType.isDir === true) {
    return 'directory'
  }
  if (nodeType.isFile === true) {
    return 'file'
  }
  return 'unknown'
}

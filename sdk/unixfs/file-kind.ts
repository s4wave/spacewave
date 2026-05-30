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
  if (info.isDir === true) {
    return 'directory'
  }
  if (info.isDir === false) {
    return 'file'
  }
  return 'unknown'
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
  if (entry.isDir === true) {
    return 'directory'
  }
  if (entry.isDir === false) {
    return 'file'
  }
  return 'unknown'
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

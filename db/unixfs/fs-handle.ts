// FSHandle is the public API for accessing a location in a FSTree.
// Implementation lives in fs-inode.ts to avoid circular imports between
// FsInode and FSHandle (they reference each other's types at runtime).
export { FSHandle, FsInode } from './fs-inode.js'
export type { AccessInodeCb } from './fs-inode.js'

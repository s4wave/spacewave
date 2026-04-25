import { FileEntry, FileEntryDetails } from './types.js'

export type SortColumn = 'name' | 'date' | 'size'
export type SortDirection = 'asc' | 'desc'

export interface SortableFileEntry extends FileEntry {
  details?: FileEntryDetails | null
}

// sortFileEntries sorts entries by the specified column and direction.
// Directories are always sorted before files.
export function sortFileEntries<T extends SortableFileEntry>(
  entries: T[],
  column: SortColumn = 'name',
  direction: SortDirection = 'asc',
): T[] {
  const multiplier = direction === 'asc' ? 1 : -1

  return [...entries].sort((a, b) => {
    // Directories first
    if (a.isDir && !b.isDir) return -1
    if (!a.isDir && b.isDir) return 1

    switch (column) {
      case 'name':
        return (
          multiplier *
          a.name.localeCompare(b.name, undefined, { numeric: true })
        )

      case 'date': {
        const aTime = a.details?.modTime?.getTime() ?? 0
        const bTime = b.details?.modTime?.getTime() ?? 0
        if (aTime === bTime)
          return a.name.localeCompare(b.name, undefined, { numeric: true })
        return multiplier * (aTime - bTime)
      }

      case 'size': {
        const aSize = a.details?.size ?? 0
        const bSize = b.details?.size ?? 0
        if (aSize === bSize)
          return a.name.localeCompare(b.name, undefined, { numeric: true })
        return multiplier * (aSize - bSize)
      }

      default:
        return 0
    }
  })
}

import { FileEntry, FileEntryDetails } from './types.js'

export type SortColumn = 'name' | 'date' | 'size'
export type SortDirection = 'asc' | 'desc'

export interface SortableFileEntry extends FileEntry {
  details?: FileEntryDetails
}

// sortFileEntries sorts entries by the specified column and direction.
// Directories are always sorted before files. A date or size sort puts the
// entries without that value after the rest, by name among themselves.
export function sortFileEntries<T extends SortableFileEntry>(
  entries: T[],
  column: SortColumn = 'name',
  direction: SortDirection = 'asc',
): T[] {
  const multiplier = direction === 'asc' ? 1 : -1
  const byName = (a: T, b: T) =>
    a.name.localeCompare(b.name, undefined, { numeric: true })
  const value = (e: T) =>
    column === 'date' ? e.details?.modTime?.getTime() : e.details?.size

  return entries.toSorted((a, b) => {
    // Directories first
    if (a.isDir && !b.isDir) return -1
    if (!a.isDir && b.isDir) return 1
    if (column === 'name') return multiplier * byName(a, b)

    // Entries without the column's value follow those that have it.
    const av = value(a)
    const bv = value(b)
    if (av === undefined || bv === undefined) {
      if (av !== undefined) return -1
      if (bv !== undefined) return 1
      return byName(a, b)
    }

    // Compare the column's value, then the name.
    return av === bv ? byName(a, b) : multiplier * (av - bv)
  })
}

export interface FileEntry {
  id: string
  name: string
  isDir?: boolean
  isSymlink?: boolean
  icon?: string
  color?: string
}

export interface FileEntryDetails {
  modTime?: Date
  size?: number
}

// FileEntryDetailsMap holds each entry's date and size by entry id. An entry
// absent from it, or without a date or size, draws a dash in that column and
// sorts after the entries that have one.
export type FileEntryDetailsMap = ReadonlyMap<string, FileEntryDetails>

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

export type GetFileEntryDetailsCallback = (
  index: number,
  entry: FileEntry,
  signal: AbortSignal,
) => Promise<FileEntryDetails | null>

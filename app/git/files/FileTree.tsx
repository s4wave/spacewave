import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import { FileList } from '@s4wave/web/editors/file-browser/FileList.js'
import type { RenderEntryCallback } from '@s4wave/web/editors/file-browser/FileListEntry.js'

// FileTreeProps are props for the FileTree component.
export interface FileTreeProps {
  entries: FileEntry[]
  onOpen: (entries: FileEntry[]) => void
  loadingId?: string | null
  autoHeight?: boolean
  renderEntry?: RenderEntryCallback
  currentPath?: string
}

// FileTree renders a directory listing using UnixFS file entries.
export function FileTree({
  entries,
  onOpen,
  loadingId,
  autoHeight,
  renderEntry,
  currentPath,
}: FileTreeProps) {
  return (
    <FileList
      entries={entries}
      onOpen={onOpen}
      loadingId={loadingId ?? undefined}
      autoHeight={autoHeight}
      renderEntry={renderEntry}
      currentPath={currentPath}
    />
  )
}

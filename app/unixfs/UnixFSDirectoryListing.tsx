import type { ComponentType, DragEvent, MouseEvent, ReactNode } from 'react'
import { LuFolderPlus, LuUpload } from 'react-icons/lu'

import { FileList } from '@s4wave/web/editors/file-browser/FileList.js'
import type { RenderEntryCallback } from '@s4wave/web/editors/file-browser/FileListEntry.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import type { AppDragEnvelope } from '@s4wave/web/dnd/app-drag.js'
import type { DownloadDragTarget } from '@s4wave/web/dnd/download-url-drag.js'
import type { ListItem } from '@s4wave/web/ui/list'

import type { UnixFSBrowserDirectoryHeaderProps } from './UnixFSBrowser.js'

interface UnixFSEmptyFolderStateProps {
  onNewFolder: () => void
  onUploadFiles: () => void
}

function UnixFSEmptyFolderState({
  onNewFolder,
  onUploadFiles,
}: UnixFSEmptyFolderStateProps) {
  return (
    <div className="flex flex-col items-center gap-3 text-center">
      <div className="border-foreground/10 bg-background/70 text-foreground-alt flex size-10 items-center justify-center rounded-md border">
        <LuUpload className="size-5" />
      </div>
      <div>
        <h2 className="text-foreground text-sm font-medium">
          This folder is empty
        </h2>
        <p className="mt-1 text-xs">Drop files here or add a folder.</p>
      </div>
      <div className="flex flex-wrap justify-center gap-2">
        <button
          type="button"
          className="border-border text-foreground hover:bg-foreground/5 inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition-colors"
          onClick={onUploadFiles}
        >
          <LuUpload className="size-3.5" />
          Upload files
        </button>
        <button
          type="button"
          className="border-border text-foreground hover:bg-foreground/5 inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition-colors"
          onClick={onNewFolder}
        >
          <LuFolderPlus className="size-3.5" />
          New folder
        </button>
      </div>
    </div>
  )
}

interface UnixFSDirectoryListingProps {
  currentPath: string
  entries: FileEntry[]
  displayEntries: FileEntry[]
  DirectoryHeader?: ComponentType<UnixFSBrowserDirectoryHeaderProps>
  loadingId?: string | null
  placeholder?: ReactNode
  renderEntry?: RenderEntryCallback
  onOpen: (entries: FileEntry[]) => void
  onContextMenu: (item: ListItem<FileEntry>, event: MouseEvent) => void
  onStateChange: (state: { selectedIds?: string[] }) => void
  onNewFolder: () => void
  onUploadFiles: () => void
  getDragEnvelope: (
    entry: FileEntry,
    context: { selectedIds: string[] },
  ) => AppDragEnvelope | null
  getDownloadDragTarget: (
    entry: FileEntry,
    context: { selectedIds: string[] },
  ) => DownloadDragTarget | null
  dropTargetEntryId: string | null
  onEntryDragOver: (
    entry: FileEntry,
    event: DragEvent<HTMLDivElement>,
  ) => boolean
  onEntryDragLeave: (entry: FileEntry, event: DragEvent<HTMLDivElement>) => void
  onEntryDrop: (entry: FileEntry, event: DragEvent<HTMLDivElement>) => void
}

export function UnixFSDirectoryListing({
  currentPath,
  entries,
  displayEntries,
  DirectoryHeader,
  loadingId,
  placeholder,
  renderEntry,
  onOpen,
  onContextMenu,
  onStateChange,
  onNewFolder,
  onUploadFiles,
  getDragEnvelope,
  getDownloadDragTarget,
  dropTargetEntryId,
  onEntryDragOver,
  onEntryDragLeave,
  onEntryDrop,
}: UnixFSDirectoryListingProps) {
  return (
    <>
      {DirectoryHeader && (
        <DirectoryHeader
          currentPath={currentPath}
          entries={entries}
          onNewFolder={onNewFolder}
          onOpen={onOpen}
          onUploadFiles={onUploadFiles}
        />
      )}
      <FileList
        entries={displayEntries}
        onOpen={onOpen}
        onContextMenu={onContextMenu}
        onStateChange={onStateChange}
        loadingId={loadingId}
        placeholder={
          placeholder ?? (
            <UnixFSEmptyFolderState
              onNewFolder={onNewFolder}
              onUploadFiles={onUploadFiles}
            />
          )
        }
        renderEntry={renderEntry}
        currentPath={currentPath}
        getDragEnvelope={getDragEnvelope}
        getDownloadDragTarget={getDownloadDragTarget}
        dropTargetEntryId={dropTargetEntryId}
        onEntryDragOver={onEntryDragOver}
        onEntryDragLeave={onEntryDragLeave}
        onEntryDrop={onEntryDrop}
      />
    </>
  )
}

import type { ComponentType, DragEvent, MouseEvent, ReactNode } from 'react'
import { LuFolderPlus, LuUpload } from '@s4wave/web/images/icons.js'

import { FileList } from '@s4wave/web/editors/file-browser/FileList.js'
import type { RenderEntryCallback } from '@s4wave/web/editors/file-browser/FileListEntry.js'
import type {
  FileEntry,
  GetFileEntryDetailsCallback,
} from '@s4wave/web/editors/file-browser/types.js'
import type { AppDragEnvelope } from '@s4wave/web/dnd/app-drag.js'
import type { DownloadDragTarget } from '@s4wave/web/dnd/download-url-drag.js'
import type { ListItem, ListStateHandle } from '@s4wave/web/ui/list'

import type { UnixFSBrowserDirectoryHeaderProps } from './UnixFSBrowser.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

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
        <LuUpload sx={stylexStyles.rule1} />
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
          <LuUpload sx={stylexStyles.rule2} />
          Upload files
        </button>
        <button
          type="button"
          className="border-border text-foreground hover:bg-foreground/5 inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition-colors"
          onClick={onNewFolder}
        >
          <LuFolderPlus sx={stylexStyles.rule2} />
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
  getEntryDetails?: GetFileEntryDetailsCallback
  onOpen: (entries: FileEntry[]) => void
  onContextMenu: (item: ListItem<FileEntry>, event: MouseEvent) => void
  // listState is the file list's selection and sort, owned by the browser.
  listState: ListStateHandle
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
  getEntryDetails,
  onOpen,
  onContextMenu,
  listState,
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
        state={listState}
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
        getEntryDetails={getEntryDetails}
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

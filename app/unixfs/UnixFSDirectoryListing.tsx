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

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
  },
  rule2: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 70%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule4: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    display: 'inline-flex',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
})

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
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <LuUpload sx={stylexStyles.rule1} />
      </div>
      <div>
        <h2 sx={stylexStyles2.rule3}>This folder is empty</h2>
        <p sx={stylexStyles2.rule4}>Drop files here or add a folder.</p>
      </div>
      <div sx={stylexStyles2.rule5}>
        <button type="button" sx={stylexStyles2.rule6} onClick={onUploadFiles}>
          <LuUpload sx={stylexStyles.rule2} />
          Upload files
        </button>
        <button type="button" sx={stylexStyles2.rule6} onClick={onNewFolder}>
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

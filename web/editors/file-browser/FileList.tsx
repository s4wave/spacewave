import React, { type DragEvent, useCallback, useMemo } from 'react'
import { LuChevronDown, LuChevronUp } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import { useStateNamespace } from '@s4wave/web/state/persist.js'
import {
  List,
  ListItem,
  ListSortFn,
  RenderHeaderProps,
  RowComponentProps,
} from '@s4wave/web/ui/list'
import type { AppDragEnvelope } from '@s4wave/web/dnd/app-drag.js'
import type { DownloadDragTarget } from '@s4wave/web/dnd/download-url-drag.js'
import type { ListState } from '@s4wave/web/ui/list/ListState.js'
import { FileEntry, GetFileEntryDetailsCallback } from './types.js'
import { SortColumn, sortFileEntries } from './FileListState.js'
import { FileListEntry } from './FileListEntry.js'
import type { RenderEntryCallback } from './FileListEntry.js'

export interface FileListDragEnvelopeContext {
  selectedIds: string[]
}

interface FileListProps {
  entries: FileEntry[]
  getEntryDetails?: GetFileEntryDetailsCallback
  onOpen?: (entries: FileEntry[]) => void
  onContextMenu?: (item: ListItem<FileEntry>, event: React.MouseEvent) => void
  onStateChange?: (state: ListState) => void
  rowHeight?: number
  headerStyle?: React.CSSProperties
  loadingId?: string | null
  autoHeight?: boolean
  renderEntry?: RenderEntryCallback
  currentPath?: string
  getDragEnvelope?: (
    entry: FileEntry,
    context: FileListDragEnvelopeContext,
  ) => AppDragEnvelope | null
  getDownloadDragTarget?: (
    entry: FileEntry,
    context: FileListDragEnvelopeContext,
  ) => DownloadDragTarget | null
  dropTargetEntryId?: string | null
  onEntryDragOver?: (
    entry: FileEntry,
    event: DragEvent<HTMLDivElement>,
  ) => boolean
  onEntryDragLeave?: (
    entry: FileEntry,
    event: DragEvent<HTMLDivElement>,
  ) => void
  onEntryDrop?: (entry: FileEntry, event: DragEvent<HTMLDivElement>) => void
}

// isSortColumn narrows a string to a valid SortColumn value.
function isSortColumn(key: string): key is SortColumn {
  return key === 'name' || key === 'date' || key === 'size'
}

// sortFn adapts sortFileEntries to the ListSortFn interface.
const sortFn: ListSortFn<FileEntry> = (items, sortKey, sortDirection) => {
  const entriesWithData = items
    .map((item) => item.data)
    .filter((entry): entry is FileEntry => entry !== undefined)
  const column = isSortColumn(sortKey) ? sortKey : 'name'
  const sorted = sortFileEntries(entriesWithData, column, sortDirection)
  return sorted.map((entry) => ({ id: entry.id, data: entry }))
}

// FileList renders a file browser list with column headers.
export function FileList({
  entries,
  getEntryDetails,
  onOpen,
  onContextMenu,
  onStateChange,
  rowHeight,
  headerStyle,
  loadingId,
  autoHeight,
  renderEntry,
  currentPath,
  getDragEnvelope,
  getDownloadDragTarget,
  dropTargetEntryId,
  onEntryDragOver,
  onEntryDragLeave,
  onEntryDrop,
}: FileListProps) {
  const namespace = useStateNamespace(['file-browser'])

  const items = useMemo<ListItem<FileEntry>[]>(
    () => entries.map((entry) => ({ id: entry.id, data: entry })),
    [entries],
  )

  const handleOpen = useMemo(
    () =>
      onOpen ?
        (openedItems: ListItem<FileEntry>[]) => {
          const fileEntries = openedItems
            .map((item) => item.data)
            .filter((entry): entry is FileEntry => entry !== undefined)
          onOpen(fileEntries)
        }
      : undefined,
    [onOpen],
  )

  const RowComponent = useCallback(
    (props: RowComponentProps<FileEntry>) => (
      <FileListEntry
        {...props}
        getEntryDetails={getEntryDetails}
        loadingId={loadingId}
        renderEntry={renderEntry}
        currentPath={currentPath}
        getDragEnvelope={getDragEnvelope}
        getDownloadDragTarget={getDownloadDragTarget}
        dropTargetEntryId={dropTargetEntryId}
        onEntryDragOver={onEntryDragOver}
        onEntryDragLeave={onEntryDragLeave}
        onEntryDrop={onEntryDrop}
      />
    ),
    [
      currentPath,
      dropTargetEntryId,
      getDragEnvelope,
      getDownloadDragTarget,
      getEntryDetails,
      loadingId,
      onEntryDragLeave,
      onEntryDragOver,
      onEntryDrop,
      renderEntry,
    ],
  )

  const renderHeader = useCallback(
    ({ state, dispatch }: RenderHeaderProps) => {
      const key = state.sortKey ?? 'name'
      const sortKey: SortColumn = isSortColumn(key) ? key : 'name'
      const sortDirection = state.sortDirection ?? 'asc'
      const SortChevron = sortDirection === 'asc' ? LuChevronDown : LuChevronUp

      const handleSort = (column: SortColumn) => {
        dispatch({ type: 'SET_SORT', sortKey: column })
      }

      return (
        <div
          className="bg-panel-header text-foreground-alt border-foreground/8 flex items-center border-b px-3 py-1.5 text-xs select-none"
          style={headerStyle}
        >
          <div
            className={cn(
              'flex min-w-[120px] flex-1 cursor-pointer items-center gap-1',
              sortKey === 'name' && 'text-foreground',
            )}
            onClick={() => handleSort('name')}
          >
            <span>Name</span>
            {sortKey === 'name' && <SortChevron className="h-3 w-3" />}
          </div>
          <div
            className={cn(
              'flex w-[140px] min-w-[100px] shrink cursor-pointer items-center gap-1 text-xs',
              sortKey === 'date' && 'text-foreground',
            )}
            onClick={() => handleSort('date')}
          >
            <span>Date Modified</span>
            {sortKey === 'date' && <SortChevron className="h-3 w-3" />}
          </div>
          <div
            className={cn(
              'flex w-[70px] min-w-[50px] shrink cursor-pointer items-center justify-end gap-1 text-xs',
              sortKey === 'size' && 'text-foreground',
            )}
            onClick={() => handleSort('size')}
          >
            {sortKey === 'size' && <SortChevron className="h-3 w-3" />}
            <span>Size</span>
          </div>
          <div className="w-8"></div>
        </div>
      )
    },
    [headerStyle],
  )

  return (
    <List
      items={items}
      rowHeight={rowHeight ?? 24}
      rowComponent={RowComponent}
      onRowDefaultAction={handleOpen}
      onRowContextMenu={onContextMenu}
      onStateChange={onStateChange}
      renderHeader={renderHeader}
      sortFn={sortFn}
      defaultSortKey="name"
      defaultSortDirection="asc"
      namespace={namespace}
      stateKey="fileList"
      autoHeight={autoHeight}
    />
  )
}

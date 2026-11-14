import {
  createContext,
  type DragEvent,
  type MouseEvent,
  type ReactNode,
  use,
  useCallback,
  useMemo,
} from 'react'
import { LuChevronDown, LuChevronUp } from 'react-icons/lu'
import {
  coarsePointerQuery,
  useMediaQuery,
} from '@s4wave/web/hooks/useMediaQuery.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useStateNamespace } from '@s4wave/web/state/persist.js'
import {
  List,
  ListItem,
  ListSortFn,
  RenderHeaderProps,
  RowComponentProps,
  useListState,
  type ListStateHandle,
} from '@s4wave/web/ui/list'
import type { AppDragEnvelope } from '@s4wave/web/dnd/app-drag.js'
import type { DownloadDragTarget } from '@s4wave/web/dnd/download-url-drag.js'
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
  onContextMenu?: (item: ListItem<FileEntry>, event: MouseEvent) => void
  // state is the List state from useFileListState.
  state: ListStateHandle
  rowHeight?: number
  loadingId?: string | null
  placeholder?: ReactNode
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

type FileListRowConfig = Pick<
  FileListProps,
  | 'getEntryDetails'
  | 'loadingId'
  | 'renderEntry'
  | 'currentPath'
  | 'getDragEnvelope'
  | 'getDownloadDragTarget'
  | 'dropTargetEntryId'
  | 'onEntryDragOver'
  | 'onEntryDragLeave'
  | 'onEntryDrop'
>

const FileListRowConfigContext = createContext<FileListRowConfig | null>(null)

// FileListRow keeps the rendered row component identity stable while hover and
// drag-drop props change during native drag sessions.
function FileListRow(props: RowComponentProps<FileEntry>) {
  const config = use(FileListRowConfigContext)
  if (!config) {
    throw new Error('FileListRow must be rendered inside FileList')
  }
  return <FileListEntry {...props} {...config} />
}

// isSortColumn narrows a string to a valid SortColumn value.
function isSortColumn(key: string): key is SortColumn {
  return key === 'name' || key === 'date' || key === 'size'
}

// sortFn adapts sortFileEntries to the ListSortFn interface.
const sortFn: ListSortFn<FileEntry> = (items, sortKey, sortDirection) => {
  const entriesWithData = items.flatMap((item) =>
    item.data ? [item.data] : [],
  )
  const column = isSortColumn(sortKey) ? sortKey : 'name'
  const sorted = sortFileEntries(entriesWithData, column, sortDirection)
  return sorted.map((entry) => ({ id: entry.id, data: entry }))
}

// useFileListState keeps a FileList's selection and sort in the file browser
// state namespace, sorted by name to start.
export function useFileListState(): ListStateHandle {
  const namespace = useStateNamespace(['file-browser'])
  return useListState(namespace, 'fileList', {
    sortKey: 'name',
    sortDirection: 'asc',
  })
}

// FileList renders a file browser list with column headers.
export function FileList({
  entries,
  getEntryDetails,
  onOpen,
  onContextMenu,
  state,
  rowHeight,
  loadingId,
  placeholder,
  renderEntry,
  currentPath,
  getDragEnvelope,
  getDownloadDragTarget,
  dropTargetEntryId,
  onEntryDragOver,
  onEntryDragLeave,
  onEntryDrop,
}: FileListProps) {
  // A coarse pointer needs taller rows for its menu targets.
  const coarsePointer = useMediaQuery(coarsePointerQuery)

  // The virtualizer reserves the full coarse-pointer menu target in each row.
  const effectiveRowHeight = Math.max(rowHeight ?? 24, coarsePointer ? 44 : 24)

  // Wrap each entry as a List item keyed by its id.
  const items = useMemo<ListItem<FileEntry>[]>(
    () => entries.map((entry) => ({ id: entry.id, data: entry })),
    [entries],
  )

  // Hand opened List items back to the caller as file entries.
  const handleOpen = useMemo(
    () =>
      onOpen
        ? (openedItems: ListItem<FileEntry>[]) => {
            const fileEntries = openedItems.flatMap((item) =>
              item.data ? [item.data] : [],
            )
            onOpen(fileEntries)
          }
        : undefined,
    [onOpen],
  )

  // Collect what every row reads, so rows rerender only when it changes.
  const rowConfig = useMemo<FileListRowConfig>(
    () => ({
      getEntryDetails,
      loadingId,
      renderEntry,
      currentPath,
      getDragEnvelope,
      getDownloadDragTarget,
      dropTargetEntryId,
      onEntryDragOver,
      onEntryDragLeave,
      onEntryDrop,
    }),
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

  // Draw the column headers the list's width shows, with the sort chevron.
  const renderHeader = useCallback(({ state, dispatch }: RenderHeaderProps) => {
    // Read the sort the header shows, defaulting to name ascending.
    const key = state.sortKey ?? 'name'
    const sortKey: SortColumn = isSortColumn(key) ? key : 'name'
    const sortDirection = state.sortDirection ?? 'asc'
    const SortChevron = sortDirection === 'asc' ? LuChevronDown : LuChevronUp

    // Sort by a column; the List flips direction when it is already sorted.
    const handleSort = (column: SortColumn) => {
      dispatch({ type: 'SET_SORT', sortKey: column })
    }

    return (
      <div className="bg-panel-header text-foreground-alt border-foreground/8 flex items-center border-b px-3.5 text-xs select-none">
        <button
          type="button"
          className={cn(
            'flex min-w-viewer-name flex-1 cursor-pointer items-center gap-1 bg-transparent px-0 py-1.5 text-left [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:py-0',
            sortKey === 'name' && 'text-foreground',
          )}
          onClick={() => handleSort('name')}
        >
          <span>Name</span>
          {sortKey === 'name' && <SortChevron aria-hidden className="size-3" />}
          {sortKey !== 'name' && (
            <span
              className={cn(
                'text-foreground-alt flex items-center gap-1',
                sortKey === 'date' && '@min-[24rem]/file-list:hidden',
                sortKey === 'size' && '@min-[15.5rem]/file-list:hidden',
              )}
            >
              by {sortKey === 'date' ? 'date' : 'size'}
              <SortChevron aria-hidden className="size-3" />
            </span>
          )}
        </button>
        <button
          type="button"
          className={cn(
            'hidden w-35 min-w-25 shrink cursor-pointer items-center gap-1 bg-transparent px-0 py-1.5 text-left text-xs @min-[24rem]/file-list:flex [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:py-0',
            sortKey === 'date' && 'text-foreground',
          )}
          onClick={() => handleSort('date')}
        >
          <span>Date Modified</span>
          {sortKey === 'date' && <SortChevron aria-hidden className="size-3" />}
        </button>
        <button
          type="button"
          className={cn(
            'hidden w-17.5 min-w-12.5 shrink cursor-pointer items-center justify-end gap-1 bg-transparent px-0 py-1.5 text-left text-xs @min-[15.5rem]/file-list:flex [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:py-0',
            sortKey === 'size' && 'text-foreground',
          )}
          onClick={() => handleSort('size')}
        >
          {sortKey === 'size' && <SortChevron aria-hidden className="size-3" />}
          <span>Size</span>
        </button>
        <div className="w-8 shrink-0 [@media(pointer:coarse)]:w-11" />
      </div>
    )
  }, [])

  return (
    <FileListRowConfigContext.Provider value={rowConfig}>
      <List
        className="@container/file-list"
        items={items}
        rowHeight={effectiveRowHeight}
        rowComponent={FileListRow}
        onRowDefaultAction={handleOpen}
        onRowContextMenu={onContextMenu}
        state={state}
        renderHeader={renderHeader}
        sortFn={sortFn}
        placeholder={placeholder}
      />
    </FileListRowConfigContext.Provider>
  )
}

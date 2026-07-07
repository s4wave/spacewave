import {
  createContext,
  type CSSProperties,
  type DragEvent,
  type MouseEvent,
  type ReactNode,
  use,
  useCallback,
  useMemo,
} from 'react'
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
  onContextMenu?: (item: ListItem<FileEntry>, event: MouseEvent) => void
  onStateChange?: (state: ListState) => void
  rowHeight?: number
  headerStyle?: CSSProperties
  loadingId?: string | null
  autoHeight?: boolean
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
  const namespace = useStateNamespace(['file-browser'])

  const items = useMemo<ListItem<FileEntry>[]>(
    () => entries.map((entry) => ({ id: entry.id, data: entry })),
    [entries],
  )

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
          <button
            type="button"
            className={cn(
              'flex min-w-[120px] flex-1 cursor-pointer items-center gap-1 bg-transparent p-0 text-left',
              sortKey === 'name' && 'text-foreground',
            )}
            onClick={() => handleSort('name')}
          >
            <span>Name</span>
            {sortKey === 'name' && <SortChevron className="size-3" />}
          </button>
          <button
            type="button"
            className={cn(
              'flex w-[140px] min-w-[100px] shrink cursor-pointer items-center gap-1 bg-transparent p-0 text-left text-xs',
              sortKey === 'date' && 'text-foreground',
            )}
            onClick={() => handleSort('date')}
          >
            <span>Date Modified</span>
            {sortKey === 'date' && <SortChevron className="size-3" />}
          </button>
          <button
            type="button"
            className={cn(
              'flex w-[70px] min-w-[50px] shrink cursor-pointer items-center justify-end gap-1 bg-transparent p-0 text-left text-xs',
              sortKey === 'size' && 'text-foreground',
            )}
            onClick={() => handleSort('size')}
          >
            {sortKey === 'size' && <SortChevron className="size-3" />}
            <span>Size</span>
          </button>
          <div className="w-8"></div>
        </div>
      )
    },
    [headerStyle],
  )

  return (
    <FileListRowConfigContext.Provider value={rowConfig}>
      <List
        items={items}
        rowHeight={rowHeight ?? 24}
        rowComponent={FileListRow}
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
        placeholder={placeholder}
      />
    </FileListRowConfigContext.Provider>
  )
}

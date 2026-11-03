import { controlTargetStyles } from '@s4wave/web/style/ControlStylesProvider.js'
import {
  createContext,
  type DragEvent,
  type MouseEvent,
  type ReactNode,
  use,
  useCallback,
  useMemo,
} from 'react'
import { LuChevronDown, LuChevronUp } from '@s4wave/web/images/icons.js'
import {
  coarsePointerQuery,
  useMediaQuery,
} from '@s4wave/web/hooks/useMediaQuery.js'

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

import * as stylex from '@stylexjs/stylex'
import {
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
} from '../../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    WebkitUserSelect: 'none',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 1.5)`,
      '@media (pointer: coarse)': 0,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-panel-header'],
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule2: {
    padding: 0,
    flex: 1,
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: '#0000',
    cursor: 'pointer',
    display: 'flex',
    textAlign: 'left',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: `calc(${themeLayout['--spacing']} * 30)`,
  },
  rule3: { color: themeColors['--color-foreground'] },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    padding: 0,
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: '#0000',
    cursor: 'pointer',
    display: 'flex',
    flexShrink: 1,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: `calc(${themeLayout['--spacing']} * 25)`,
    width: `calc(${themeLayout['--spacing']} * 35)`,
  },
  rule6: {
    padding: 0,
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: '#0000',
    cursor: 'pointer',
    display: 'flex',
    flexShrink: 1,
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'flex-end',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: `calc(${themeLayout['--spacing']} * 12.5)`,
    width: `calc(${themeLayout['--spacing']} * 17.5)`,
  },
  rule7: { width: `calc(${themeLayout['--spacing']} * 8)` },
})

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
  const coarsePointer = useMediaQuery(coarsePointerQuery)

  // The virtualizer reserves the full coarse-pointer menu target in each row.
  const effectiveRowHeight = Math.max(rowHeight ?? 24, coarsePointer ? 44 : 24)

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

  const renderHeader = useCallback(({ state, dispatch }: RenderHeaderProps) => {
    const key = state.sortKey ?? 'name'
    const sortKey: SortColumn = isSortColumn(key) ? key : 'name'
    const sortDirection = state.sortDirection ?? 'asc'
    const SortChevron = sortDirection === 'asc' ? LuChevronDown : LuChevronUp

    const handleSort = (column: SortColumn) => {
      dispatch({ type: 'SET_SORT', sortKey: column })
    }

    return (
      <div sx={stylexStyles.rule1}>
        <button
          type="button"
          sx={[
            stylexStyles.rule2,
            sortKey === 'name' && stylexStyles.rule3,
            controlTargetStyles.minimum,
          ]}
          onClick={() => handleSort('name')}
        >
          <span>Name</span>
          {sortKey === 'name' && <SortChevron sx={stylexStyles.rule4} />}
        </button>
        <button
          type="button"
          sx={[
            stylexStyles.rule5,
            sortKey === 'date' && stylexStyles.rule3,
            controlTargetStyles.minimum,
          ]}
          onClick={() => handleSort('date')}
        >
          <span>Date Modified</span>
          {sortKey === 'date' && <SortChevron sx={stylexStyles.rule4} />}
        </button>
        <button
          type="button"
          sx={[
            stylexStyles.rule6,
            sortKey === 'size' && stylexStyles.rule3,
            controlTargetStyles.minimum,
          ]}
          onClick={() => handleSort('size')}
        >
          {sortKey === 'size' && <SortChevron sx={stylexStyles.rule4} />}
          <span>Size</span>
        </button>
        <div sx={stylexStyles.rule7}></div>
      </div>
    )
  }, [])

  return (
    <FileListRowConfigContext.Provider value={rowConfig}>
      <List
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

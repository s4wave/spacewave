import {
  type ReactNode,
  useCallback,
  use,
  useEffect,
  useRef,
  type DragEvent,
  type KeyboardEvent,
  type MouseEvent,
} from 'react'
import { format } from 'date-fns'
import { LuFolder, LuFile, LuEllipsis } from 'react-icons/lu'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import { RowComponentProps, ListStateContext } from '@s4wave/web/ui/list'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import {
  clearActiveAppDragEnvelope,
  type AppDragEnvelope,
  writeAppDragEnvelope,
} from '@s4wave/web/dnd/app-drag.js'
import {
  type DownloadDragTarget,
  writeDownloadURLDragTarget,
} from '@s4wave/web/dnd/download-url-drag.js'
import {
  FileEntry,
  FileEntryDetails,
  GetFileEntryDetailsCallback,
} from './types.js'
import type { FileListDragEnvelopeContext } from './FileList.js'

function isEditableElement(el: Element | null): el is HTMLElement {
  return (
    el instanceof HTMLElement &&
    (el.matches('input, textarea, select, [contenteditable="true"]') ||
      el.isContentEditable)
  )
}

function formatBytes(bytes: number, decimals = 0): string {
  if (bytes === 0) return '0 Bytes'
  const k = 1024
  const dm = decimals < 0 ? 0 : decimals
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i]
}

// RenderEntryCallback receives the default row node and entry context.
export type RenderEntryCallback = (props: {
  entry: FileEntry
  defaultNode: ReactNode
  path: string
}) => ReactNode

interface FileListEntryProps extends RowComponentProps<FileEntry> {
  getEntryDetails?: GetFileEntryDetailsCallback
  loadingId?: string | null
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

type EntryDragProps = Pick<
  FileListEntryProps,
  | 'getDragEnvelope'
  | 'getDownloadDragTarget'
  | 'onEntryDragOver'
  | 'onEntryDragLeave'
  | 'onEntryDrop'
>

// useEntryDrag binds the drag source and drop target handlers of a row.
function useEntryDrag(
  entry: FileEntry,
  selectedIds: string[],
  {
    getDragEnvelope,
    getDownloadDragTarget,
    onEntryDragOver,
    onEntryDragLeave,
    onEntryDrop,
  }: EntryDragProps,
) {
  // handleDragStart builds the drag payloads for the current selection and
  // cancels the drag when the entry offers none.
  const handleDragStart = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      const selection = { selectedIds }
      const dragEnvelope = getDragEnvelope?.(entry, selection) ?? null
      const downloadDragTarget =
        getDownloadDragTarget?.(entry, selection) ?? null
      if (!dragEnvelope && !downloadDragTarget) {
        e.preventDefault()
        return
      }

      if (dragEnvelope) {
        writeAppDragEnvelope(e.dataTransfer, dragEnvelope)
      }
      if (downloadDragTarget) {
        writeDownloadURLDragTarget(
          e.dataTransfer,
          downloadDragTarget,
          window.location.href,
        )
      }
      e.dataTransfer.effectAllowed = 'copyMove'
    },
    [selectedIds, entry, getDownloadDragTarget, getDragEnvelope],
  )

  const handleDragEnd = useCallback(() => {
    clearActiveAppDragEnvelope()
  }, [])

  const handleDragOver = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      if (!onEntryDragOver?.(entry, e)) return
      e.preventDefault()
      e.stopPropagation()
      e.dataTransfer.dropEffect = 'move'
    },
    [entry, onEntryDragOver],
  )

  const handleDragLeave = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      onEntryDragLeave?.(entry, e)
    },
    [entry, onEntryDragLeave],
  )

  const handleDrop = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      if (!onEntryDragOver?.(entry, e)) return
      e.preventDefault()
      e.stopPropagation()
      onEntryDrop?.(entry, e)
    },
    [entry, onEntryDragOver, onEntryDrop],
  )

  return {
    draggable: !!getDragEnvelope || !!getDownloadDragTarget,
    onDragStart: handleDragStart,
    onDragEnd: handleDragEnd,
    onDragOver: handleDragOver,
    onDragLeave: handleDragLeave,
    onDrop: handleDrop,
  }
}

// useFocusRow moves DOM focus to the row when it becomes the focused row,
// unless the user is typing in an input.
function useFocusRow(focused: boolean) {
  const divRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!focused || !divRef.current) return
    const hasInput = divRef.current.querySelector(
      'input, textarea, select, [contenteditable="true"]',
    )
    if (hasInput) return
    const active = document.activeElement
    if (isEditableElement(active) && !divRef.current.contains(active)) return
    divRef.current.focus()
  }, [focused])

  return divRef
}

// entrySizeLabel formats the size column of an entry.
function entrySizeLabel(
  entry: FileEntry,
  details: FileEntryDetails | null | undefined,
): string {
  if (!details?.size || entry.isSymlink) return '—'
  if (entry.isDir) return String(details.size)
  return formatBytes(details.size, 0)
}

type RowHandlerProps = Pick<
  FileListEntryProps,
  'item' | 'itemIndex' | 'onRowClick' | 'onContextMenu'
>

// useRowHandlers binds the selection and context menu handlers of a row.
function useRowHandlers(
  item: RowHandlerProps['item'],
  itemIndex: RowHandlerProps['itemIndex'],
  onRowClick: RowHandlerProps['onRowClick'],
  onContextMenu: RowHandlerProps['onContextMenu'],
) {
  const select = useCallback(
    (e: MouseEvent) => {
      onRowClick?.(itemIndex, item, e, 1)
    },
    [itemIndex, item, onRowClick],
  )

  const doubleClick = useCallback(
    (e: MouseEvent) => {
      onRowClick?.(itemIndex, item, e, 2)
    },
    [itemIndex, item, onRowClick],
  )

  const keyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key !== 'Enter' && e.key !== ' ') return
      e.preventDefault()
      onRowClick?.(itemIndex, item, e as unknown as MouseEvent, 1)
    },
    [itemIndex, item, onRowClick],
  )

  const contextMenu = useCallback(
    (e: MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      onContextMenu?.(itemIndex, item, e)
    },
    [itemIndex, item, onContextMenu],
  )

  const dotsClick = useCallback(
    (e: MouseEvent) => {
      e.stopPropagation()
      onContextMenu?.(itemIndex, item, e)
    },
    [itemIndex, item, onContextMenu],
  )

  return { select, doubleClick, keyDown, contextMenu, dotsClick }
}

// FileEntryName renders the default icon and name of an entry.
function FileEntryName({
  entry,
  loading,
  selected,
}: {
  entry: FileEntry
  loading: boolean
  selected: boolean
}) {
  let icon = (
    <LuFile
      className={cn(
        'file-entry-color size-4 shrink-0',
        selected ? 'text-foreground' : 'text-foreground-alt/60',
      )}
      style={{ '--file-entry-color': entry.color }}
    />
  )
  if (entry.isDir) {
    icon = (
      <LuFolder
        className={cn(
          'file-entry-color size-4 shrink-0',
          selected ? 'text-brand' : 'text-foreground-alt/80',
        )}
        style={{ '--file-entry-color': entry.color }}
      />
    )
  }
  if (loading) icon = <Spinner variant="brand" className="shrink-0" />

  return (
    <div className="flex min-w-30 flex-1 items-center gap-2 overflow-hidden">
      {icon}
      <span className="truncate">{entry.name || entry.id}</span>
    </div>
  )
}

// FileListEntry renders a file browser row with icon, name, date, and size.
export function FileListEntry(props: FileListEntryProps) {
  const entry = props.item.data
  if (!entry) return null

  return <FileEntryRow {...props} entry={entry} />
}

// FileEntryRow renders the row of an entry that exists.
function FileEntryRow({
  entry,
  item,
  itemIndex,
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
  onRowClick,
  onContextMenu,
  style,
  ariaAttributes,
}: FileListEntryProps & { entry: FileEntry }) {
  const context = use(ListStateContext)
  const selectedIds = context?.selectedIds
  const selected = selectedIds?.includes(entry.id) ?? false
  const focused = itemIndex === context?.focusedIndex
  const divRef = useFocusRow(focused)
  const row = useRowHandlers(item, itemIndex, onRowClick, onContextMenu)
  const drag = useEntryDrag(entry, selectedIds ?? [], {
    getDragEnvelope,
    getDownloadDragTarget,
    onEntryDragOver,
    onEntryDragLeave,
    onEntryDrop,
  })

  const fetchDetails = useCallback(
    (signal: AbortSignal) => {
      if (!getEntryDetails) {
        return Promise.resolve(null)
      }
      return getEntryDetails(itemIndex, entry, signal)
    },
    [entry, itemIndex, getEntryDetails],
  )

  const { data: entryDetails } = usePromise(fetchDetails)

  const defaultNode = (
    <FileEntryName
      entry={entry}
      loading={entry.id === loadingId}
      selected={selected}
    />
  )

  return (
    <div
      ref={divRef}
      role="row"
      tabIndex={focused ? 0 : -1}
      aria-selected={selected || undefined}
      aria-posinset={ariaAttributes['aria-posinset']}
      aria-setsize={ariaAttributes['aria-setsize']}
      style={style}
      className={cn(
        'group relative flex items-center px-3 py-1.5 text-xs [@media(pointer:coarse)]:py-0',
        'cursor-pointer transition-colors select-none',
        selected
          ? 'bg-brand/10 text-foreground'
          : 'text-foreground/90 hover:bg-foreground/5',
        focused && !selected && 'ring-brand/25 ring-1 ring-inset',
        entry.id === dropTargetEntryId &&
          'bg-brand/10 ring-brand/40 ring-1 ring-inset',
        style['--list-row-height'] !== undefined && 'list-row-height',
      )}
      draggable={drag.draggable}
      onClick={row.select}
      onKeyDown={row.keyDown}
      onDoubleClick={row.doubleClick}
      onContextMenu={row.contextMenu}
      onDragStart={drag.onDragStart}
      onDragEnd={drag.onDragEnd}
      onDragOver={drag.onDragOver}
      onDragLeave={drag.onDragLeave}
      onDrop={drag.onDrop}
    >
      {selected && (
        <span className="bg-brand/80 absolute top-1 bottom-1 left-0 w-0.5 rounded-r" />
      )}
      {renderEntry
        ? renderEntry({
            entry,
            defaultNode,
            path: currentPath ?? '/',
          })
        : defaultNode}
      <div className="text-foreground-alt/50 w-35 min-w-25 shrink text-xs">
        {entryDetails?.modTime
          ? format(entryDetails.modTime, 'MMM dd, yyyy')
          : '—'}
      </div>
      <div className="text-foreground-alt/50 w-17.5 min-w-12.5 shrink text-right text-xs">
        {entrySizeLabel(entry, entryDetails)}
      </div>
      <button
        type="button"
        aria-label={`More actions for ${entry.name || entry.id}`}
        className="flex h-full w-8 shrink-0 items-center justify-center [@media(pointer:coarse)]:size-11"
        onClick={row.dotsClick}
        onDoubleClick={(e) => {
          e.stopPropagation()
          row.dotsClick(e)
        }}
        onContextMenu={row.dotsClick}
      >
        <LuEllipsis className="text-foreground-alt size-4 opacity-0 transition-opacity group-hover:opacity-100 [@media(pointer:coarse)]:opacity-100" />
      </button>
    </div>
  )
}

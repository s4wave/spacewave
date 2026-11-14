import {
  type ReactNode,
  useCallback,
  use,
  type DragEvent,
  type MouseEvent,
} from 'react'
import { format } from 'date-fns'
import { LuFolder, LuFile, LuEllipsis } from 'react-icons/lu'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  ListRow,
  ListStateContext,
  type RowComponentProps,
} from '@s4wave/web/ui/list'
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

function formatBytes(bytes: number, decimals = 0): string {
  // Zero has no unit to scale.
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
      // Ask the caller what this entry and the selection carry when dragged.
      const selection = { selectedIds }
      const dragEnvelope = getDragEnvelope?.(entry, selection) ?? null
      const downloadDragTarget =
        getDownloadDragTarget?.(entry, selection) ?? null
      if (!dragEnvelope && !downloadDragTarget) {
        e.preventDefault()
        return
      }

      // Write each payload the caller offered.
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

  // Drop the in-app envelope once the drag ends either way.
  const handleDragEnd = useCallback(() => {
    clearActiveAppDragEnvelope()
  }, [])

  // Accept a drag over a folder the caller says can take it.
  const handleDragOver = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      if (!onEntryDragOver?.(entry, e)) return
      e.preventDefault()
      e.stopPropagation()
      e.dataTransfer.dropEffect = 'move'
    },
    [entry, onEntryDragOver],
  )

  // Tell the caller the drag left this entry.
  const handleDragLeave = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      onEntryDragLeave?.(entry, e)
    },
    [entry, onEntryDragLeave],
  )

  // Drop onto this entry only when the caller accepts it.
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
  'item' | 'itemIndex' | 'onContextMenu'
>

// useMorePress opens the row's menu from More without the click reaching the
// row, which would select it or, on a double click, open it.
function useMorePress(
  item: RowHandlerProps['item'],
  itemIndex: RowHandlerProps['itemIndex'],
  onContextMenu: RowHandlerProps['onContextMenu'],
) {
  return useCallback(
    (e: MouseEvent) => {
      e.stopPropagation()
      onContextMenu?.(itemIndex, item, e)
    },
    [itemIndex, item, onContextMenu],
  )
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
      aria-hidden
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
        aria-hidden
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
    <div
      role="cell"
      className="min-w-viewer-name flex flex-1 items-center gap-2 overflow-hidden"
    >
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
  // Read the row's selection from the List.
  const context = use(ListStateContext)
  const selectedIds = context?.selectedIds
  const selected = selectedIds?.includes(entry.id) ?? false
  const more = useMorePress(item, itemIndex, onContextMenu)
  const drag = useEntryDrag(entry, selectedIds ?? [], {
    getDragEnvelope,
    getDownloadDragTarget,
    onEntryDragOver,
    onEntryDragLeave,
    onEntryDrop,
  })

  // Load the entry's date and size when the caller provides them.
  const fetchDetails = useCallback(
    (signal: AbortSignal) => {
      if (!getEntryDetails) {
        return Promise.resolve(null)
      }
      return getEntryDetails(itemIndex, entry, signal)
    },
    [entry, itemIndex, getEntryDetails],
  )

  // Show the details once they arrive; the cells show a dash until then.
  const { data: entryDetails } = usePromise(fetchDetails)

  // Draw the icon and name unless the caller renders the entry.
  const defaultNode = (
    <FileEntryName
      entry={entry}
      loading={entry.id === loadingId}
      selected={selected}
    />
  )

  return (
    <ListRow
      item={item}
      itemIndex={itemIndex}
      onRowClick={onRowClick}
      onContextMenu={onContextMenu}
      style={style}
      ariaAttributes={ariaAttributes}
      dropTarget={entry.id === dropTargetEntryId}
      draggable={drag.draggable}
      onDragStart={drag.onDragStart}
      onDragEnd={drag.onDragEnd}
      onDragOver={drag.onDragOver}
      onDragLeave={drag.onDragLeave}
      onDrop={drag.onDrop}
    >
      {renderEntry
        ? renderEntry({
            entry,
            defaultNode,
            path: currentPath ?? '/',
          })
        : defaultNode}
      <div
        role="cell"
        className="text-foreground-alt/60 hidden w-35 min-w-25 shrink text-xs @min-[24rem]/file-list:block"
      >
        {entryDetails?.modTime
          ? format(entryDetails.modTime, 'MMM dd, yyyy')
          : '—'}
      </div>
      <div
        role="cell"
        className="text-foreground-alt/60 hidden w-17.5 min-w-12.5 shrink text-right text-xs @min-[15.5rem]/file-list:block"
      >
        {entrySizeLabel(entry, entryDetails)}
      </div>
      <button
        type="button"
        aria-label={`More actions for ${entry.name || entry.id}`}
        className="-my-1.5 flex w-8 shrink-0 items-center justify-center self-stretch [@media(pointer:coarse)]:my-0 [@media(pointer:coarse)]:size-11"
        onClick={more}
        onDoubleClick={more}
        onContextMenu={more}
      >
        <LuEllipsis
          aria-hidden
          className="text-foreground-alt size-4 opacity-0 transition-opacity group-hover:opacity-100 [@media(pointer:coarse)]:opacity-100"
        />
      </button>
    </ListRow>
  )
}

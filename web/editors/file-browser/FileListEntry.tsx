import {
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  type DragEvent,
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
import { FileEntry, GetFileEntryDetailsCallback } from './types.js'
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

// FileListEntry renders a file browser row with icon, name, date, and size.
export function FileListEntry({
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
}: FileListEntryProps) {
  const entry = item.data
  const isEntryLoading = entry ? entry.id === loadingId : false

  const handleClick = useCallback(
    (e: MouseEvent) => {
      onRowClick?.(itemIndex, item, e, 1)
    },
    [itemIndex, item, onRowClick],
  )

  const handleDoubleClick = useCallback(
    (e: MouseEvent) => {
      onRowClick?.(itemIndex, item, e, 2)
    },
    [itemIndex, item, onRowClick],
  )

  const handleContextMenu = useCallback(
    (e: MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      onContextMenu?.(itemIndex, item, e)
    },
    [itemIndex, item, onContextMenu],
  )

  const handleDotsClick = useCallback(
    (e: MouseEvent) => {
      e.stopPropagation()
      onContextMenu?.(itemIndex, item, e)
    },
    [itemIndex, item, onContextMenu],
  )

  const context = useContext(ListStateContext)
  const selected =
    entry ? (context?.selectedIds?.includes(entry.id) ?? false) : false
  const focused = itemIndex === context?.focusedIndex

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

  const fetchDetails = useCallback(
    (signal: AbortSignal) => {
      if (!getEntryDetails || !entry) {
        return Promise.resolve(null)
      }
      return getEntryDetails(itemIndex, entry, signal)
    },
    [entry, itemIndex, getEntryDetails],
  )

  const { data: entryDetails } = usePromise(fetchDetails)
  const dragEnvelope = useMemo(
    () =>
      entry ?
        (getDragEnvelope?.(entry, {
          selectedIds: context?.selectedIds ?? [],
        }) ?? null)
      : null,
    [context?.selectedIds, entry, getDragEnvelope],
  )
  const downloadDragTarget = useMemo(
    () =>
      entry ?
        (getDownloadDragTarget?.(entry, {
          selectedIds: context?.selectedIds ?? [],
        }) ?? null)
      : null,
    [context?.selectedIds, entry, getDownloadDragTarget],
  )

  const iconStyle = useMemo(
    () => (entry?.color ? { color: entry.color } : undefined),
    [entry],
  )
  const isDropTargetActive = entry?.id === dropTargetEntryId

  const handleDragStart = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      if (!dragEnvelope && !downloadDragTarget) return
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
    [downloadDragTarget, dragEnvelope],
  )

  const handleDragEnd = useCallback(() => {
    clearActiveAppDragEnvelope()
  }, [])

  const handleDragOver = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      if (!entry) return
      if (!onEntryDragOver?.(entry, e)) return
      e.preventDefault()
      e.stopPropagation()
      e.dataTransfer.dropEffect = 'move'
    },
    [entry, onEntryDragOver],
  )

  const handleDragLeave = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      if (!entry) return
      onEntryDragLeave?.(entry, e)
    },
    [entry, onEntryDragLeave],
  )

  const handleDrop = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      if (!entry) return
      if (!onEntryDragOver?.(entry, e)) return
      e.preventDefault()
      e.stopPropagation()
      onEntryDrop?.(entry, e)
    },
    [entry, onEntryDragOver, onEntryDrop],
  )

  if (!entry) return null

  const defaultNode = (
    <div className="flex min-w-[120px] flex-1 items-center gap-2 overflow-hidden">
      {isEntryLoading ?
        <Spinner className="text-brand shrink-0" />
      : entry.isDir ?
        <LuFolder
          className="text-file-folder-icon h-4 w-4 shrink-0"
          style={iconStyle}
        />
      : <LuFile
          className="text-foreground-alt h-4 w-4 shrink-0"
          style={iconStyle}
        />
      }
      <span className="truncate">{entry.name || entry.id}</span>
    </div>
  )

  const rowContent = (
    <>
      {renderEntry ?
        renderEntry({
          entry,
          defaultNode,
          path: currentPath ?? '/',
        })
      : defaultNode}
      <div className="text-foreground-alt w-[140px] min-w-[100px] shrink text-xs opacity-70">
        {entryDetails?.modTime ?
          format(entryDetails.modTime, 'MMM dd, yyyy')
        : '—'}
      </div>
      <div className="text-foreground-alt w-[70px] min-w-[50px] shrink text-right text-xs opacity-70">
        {entryDetails?.size && !entry.isSymlink ?
          entry.isDir ?
            entryDetails.size
          : formatBytes(entryDetails.size, 0)
        : '—'}
      </div>
      <div
        className="flex h-full w-8 items-center justify-center"
        onClick={handleDotsClick}
        onDoubleClick={(e) => {
          e.stopPropagation()
          handleDotsClick(e)
        }}
        onContextMenu={handleDotsClick}
      >
        <LuEllipsis className="text-foreground-alt h-4 w-4 opacity-0 transition-opacity group-hover:opacity-100" />
      </div>
    </>
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
        'group text-file-browser-row flex items-center px-3 py-1.5',
        'hover:bg-outliner-selected-highlight cursor-pointer transition-colors select-none',
        selected && 'bg-ui-selected hover:bg-ui-selected',
        itemIndex % 2 === 1 && !selected && 'bg-file-row-alternate',
        focused && 'ring-ui-outline-active ring-1 ring-inset',
        isDropTargetActive && 'bg-brand/10 ring-brand/40 ring-1 ring-inset',
      )}
      draggable={dragEnvelope !== null || downloadDragTarget !== null}
      onClick={handleClick}
      onDoubleClick={handleDoubleClick}
      onContextMenu={handleContextMenu}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {rowContent}
    </div>
  )
}

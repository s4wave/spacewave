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
import { LuFolder, LuFile, LuEllipsis } from '@s4wave/web/images/icons.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

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

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
} from '../../style/tokens.stylex.js'

import { motion as themeMotion } from '../../style/tokens.stylex.js'
import { groupMarker } from '../../style/relations.stylex.js'

import { groupMarker as groupMarker3 } from '../../style/relations.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 1.5)`,
      '@media (pointer: coarse)': 0,
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-ring-shadow':
      'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    '--tw-ring-inset': 'inset',
  },
  rule3: {
    position: 'relative',
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 1.5)`,
      '@media (pointer: coarse)': 0,
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    position: 'relative',
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 1.5)`,
      '@media (pointer: coarse)': 0,
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 90%, transparent)`,
    '--tw-ring-shadow':
      'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    '--tw-ring-inset': 'inset',
  },
  rule5: {
    position: 'relative',
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 1.5)`,
      '@media (pointer: coarse)': 0,
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 90%, transparent)`,
    '--tw-ring-shadow':
      'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 25%, transparent)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    '--tw-ring-inset': 'inset',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule6: {
    position: 'relative',
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 1.5)`,
      '@media (pointer: coarse)': 0,
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 90%, transparent)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
    opacity: {
      default: 0,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]: 1,
      },
      '@media (pointer: coarse)': 1,
    },
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    color: 'var(--file-entry-color)',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
  rule2: { color: themeColors['--color-foreground'] },
  rule3: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule4: { color: themeColors['--color-brand'] },
  rule5: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    minWidth: `calc(${themeLayout['--spacing']} * 30)`,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    overflow: 'hidden',
  },
  rule2: { textOverflow: 'ellipsis', whiteSpace: 'nowrap', overflow: 'hidden' },
  rule3: {
    position: 'absolute',
    top: themeLayout['--spacing'],
    bottom: themeLayout['--spacing'],
    left: 0,
    width: `calc(${themeLayout['--spacing']} * .5)`,
    borderTopRightRadius: '.25rem',
    borderBottomRightRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 35)`,
    minWidth: `calc(${themeLayout['--spacing']} * 25)`,
    flexShrink: 1,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 17.5)`,
    minWidth: `calc(${themeLayout['--spacing']} * 12.5)`,
    flexShrink: 1,
    textAlign: 'right',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule6: {
    display: 'flex',
    height: {
      default: '100%',
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    width: {
      default: `calc(${themeLayout['--spacing']} * 8)`,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
  },
})

const stylexStyles = stylex.create({ rule1: { flexShrink: 0 } })

function isEditableElement(el: Element | null): el is HTMLElement {
  return (
    el instanceof HTMLElement &&
    (el.matches('input, textarea, select, [contenteditable="true"]') ||
      el.isContentEditable)
  )
}

function formatBytes(bytes: number, decimals = 0): string {
  // Choose a binary size unit and round the displayed quantity.
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
      // Resolve the drag payloads offered for this row's current selection.
      const selection = { selectedIds }
      const dragEnvelope = getDragEnvelope?.(entry, selection) ?? null
      const downloadDragTarget =
        getDownloadDragTarget?.(entry, selection) ?? null
      if (!dragEnvelope && !downloadDragTarget) {
        e.preventDefault()
        return
      }

      // Publish the supported app and download payloads to the native drag operation.
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

  // Clear the app drag payload when the row's drag ends.
  const handleDragEnd = useCallback(() => {
    clearActiveAppDragEnvelope()
  }, [])

  // Accept a move only when this entry's drop handler allows it.
  const handleDragOver = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      if (!onEntryDragOver?.(entry, e)) return
      e.preventDefault()
      e.stopPropagation()
      e.dataTransfer.dropEffect = 'move'
    },
    [entry, onEntryDragOver],
  )

  // Notify the entry when the drag leaves its drop target.
  const handleDragLeave = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      onEntryDragLeave?.(entry, e)
    },
    [entry, onEntryDragLeave],
  )

  // Deliver an accepted drop through the entry's existing callback.
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
    // Focus the active row without stealing focus from an editable control.
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
  // Deliver a single row click with its current item and index.
  const select = useCallback(
    (e: MouseEvent) => {
      onRowClick?.(itemIndex, item, e, 1)
    },
    [itemIndex, item, onRowClick],
  )

  // Deliver the row's default action with a double-click count.
  const doubleClick = useCallback(
    (e: MouseEvent) => {
      onRowClick?.(itemIndex, item, e, 2)
    },
    [itemIndex, item, onRowClick],
  )

  // Deliver keyboard activation through the same row-click contract.
  const keyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key !== 'Enter' && e.key !== ' ') return
      e.preventDefault()
      onRowClick?.(itemIndex, item, e as unknown as MouseEvent, 1)
    },
    [itemIndex, item, onRowClick],
  )

  // Open the row context menu without also selecting its parent.
  const contextMenu = useCallback(
    (e: MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      onContextMenu?.(itemIndex, item, e)
    },
    [itemIndex, item, onContextMenu],
  )

  // Open the row menu from its trailing control without bubbling the click.
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
      sx={[
        stylexStyles3.rule1,
        selected ? stylexStyles3.rule2 : stylexStyles3.rule3,
      ]}
      style={{ '--file-entry-color': entry.color }}
    />
  )
  if (entry.isDir) {
    icon = (
      <LuFolder
        sx={[
          stylexStyles3.rule1,
          selected ? stylexStyles3.rule4 : stylexStyles3.rule5,
        ]}
        style={{ '--file-entry-color': entry.color }}
      />
    )
  }
  if (loading) icon = <Spinner variant="brand" sx={stylexStyles.rule1} />

  return (
    <div sx={stylexStyles2.rule1}>
      {icon}
      <span sx={stylexStyles2.rule2}>{entry.name || entry.id}</span>
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
  // Bind the row's selection, focus and drag behavior to the shared list state.
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

  // Load optional details through the caller's cancellable entry reader.
  const fetchDetails = useCallback(
    (signal: AbortSignal) => {
      if (!getEntryDetails) {
        return Promise.resolve(null)
      }
      return getEntryDetails(itemIndex, entry, signal)
    },
    [entry, itemIndex, getEntryDetails],
  )

  // Use the current details result without replacing the row's identity.
  const { data: entryDetails } = usePromise(fetchDetails)

  // Render the ordinary file name when the caller provides no replacement.
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
      sx={fileEntryStyles(selected, focused, entry.id === dropTargetEntryId)}
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
      {selected && <span sx={stylexStyles2.rule3} />}
      {renderEntry
        ? renderEntry({
            entry,
            defaultNode,
            path: currentPath ?? '/',
          })
        : defaultNode}
      <div sx={stylexStyles2.rule4}>
        {entryDetails?.modTime
          ? format(entryDetails.modTime, 'MMM dd, yyyy')
          : '—'}
      </div>
      <div sx={stylexStyles2.rule5}>{entrySizeLabel(entry, entryDetails)}</div>
      <button
        type="button"
        aria-label={`More actions for ${entry.name || entry.id}`}
        sx={stylexStyles2.rule6}
        onClick={row.dotsClick}
        onDoubleClick={(e) => {
          e.stopPropagation()
          row.dotsClick(e)
        }}
        onContextMenu={row.dotsClick}
      >
        <LuEllipsis sx={stylexStyles4.rule1} />
      </button>
    </div>
  )
}

// fileEntryStyles keeps drop emphasis ahead of focus and selection styling.
function fileEntryStyles(
  selected: boolean,
  focused: boolean,
  dropTarget: boolean,
): stylex.StyleXStyles {
  // Selected rows suppress the ordinary focus ring; drop emphasis still wins.
  if (selected)
    return [
      dropTarget ? stylexStyles5.rule1 : stylexStyles5.rule3,
      groupMarker3,
    ]

  // Unselected drop targets use their stronger ring independently of keyboard focus.
  if (dropTarget) return [stylexStyles5.rule4, groupMarker3]

  // Ordinary rows retain the focused and hovered outcomes from the list's state.
  return [focused ? stylexStyles5.rule5 : stylexStyles5.rule6, groupMarker3]
}

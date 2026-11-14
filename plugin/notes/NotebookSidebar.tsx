import { useCallback, type ReactNode } from 'react'

import type { NotebookSource } from './proto/notebook.pb.js'
import type { SourceEditState } from './useNotebookViewerState.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useStateAtom, type StateNamespace } from '@s4wave/web/state/index.js'
import {
  LuChevronRight,
  LuChevronDown,
  LuFolder,
  LuPlus,
  LuTrash2,
  LuArrowUp,
  LuArrowDown,
} from 'react-icons/lu'

interface NotebookSidebarProps {
  sources: NotebookSource[]
  selectedSource: number
  onSelectSource: (index: number) => void
  onAddSource?: () => void
  onRemoveSource?: (index: number) => void
  onMoveSource?: (index: number, delta: -1 | 1) => void
  // sourceEdit is the source edit control's state; the sidebar shows a
  // refused edit and a failed Reorder, and disables edits while one sends.
  sourceEdit?: SourceEditState
  onRetryReorder?: () => void
  namespace: StateNamespace
}

/** NotebookSidebar shows the source list from the Notebook. */
function NotebookSidebar({
  sources,
  selectedSource,
  onSelectSource,
  onAddSource,
  onRemoveSource,
  onMoveSource,
  sourceEdit = { status: 'idle' },
  onRetryReorder,
  namespace,
}: NotebookSidebarProps) {
  const sending = sourceEdit.status === 'sending'
  const [expandedSources, setExpandedSources] = useStateAtom<
    Record<number, boolean>
  >(namespace, 'expandedSources', {})

  const toggleExpanded = useCallback(
    (index: number) => {
      setExpandedSources((prev) => ({
        ...prev,
        [index]: !prev[index],
      }))
    },
    [setExpandedSources],
  )

  // A refused edit and a failed Reorder show by the list; a failed Add or
  // Remove shows in its own dialog.
  let editNotice: ReactNode = null
  if (sourceEdit.status === 'changed') {
    editNotice = (
      <div
        role="status"
        className="text-muted-foreground border-border border-b px-3 py-1.5 text-xs"
      >
        The sources changed before this edit, so it was not applied.
      </div>
    )
  } else if (
    sourceEdit.status === 'failed' &&
    sourceEdit.edit.kind === 'reorder'
  ) {
    editNotice = (
      <div
        role="alert"
        className="text-destructive border-border flex items-center justify-between gap-2 border-b px-3 py-1.5 text-xs"
      >
        <span className="min-w-0">
          Could not move the source: {sourceEdit.error}
        </span>
        <button
          type="button"
          className="text-foreground hover:bg-list-hover-background min-h-11 shrink-0 rounded px-2 font-medium md:pointer-fine:min-h-0 md:pointer-fine:py-0.5"
          onClick={onRetryReorder}
        >
          Try again
        </button>
      </div>
    )
  }

  if (sources.length === 0) {
    return (
      <div className="text-muted-foreground flex h-full flex-col items-center justify-center gap-3 p-4 text-center text-xs">
        {editNotice}
        <span>No sources configured</span>
        <button
          type="button"
          className="border-border text-foreground hover:bg-list-hover-background min-h-11 rounded-md border px-3 py-1.5 text-xs font-medium md:pointer-fine:min-h-0"
          onClick={onAddSource}
        >
          Add source
        </button>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col overflow-y-auto">
      <div className="text-foreground-alt border-border border-b px-3 py-2 text-xs font-medium tracking-wide uppercase">
        <div className="flex items-center justify-between gap-2">
          <span>Sources</span>
          <button
            type="button"
            className="hover:bg-list-hover-background text-foreground-alt hover:text-foreground flex size-11 items-center justify-center rounded md:pointer-fine:size-auto md:pointer-fine:p-1"
            title="Add source"
            onClick={onAddSource}
          >
            <LuPlus className="size-3" />
          </button>
        </div>
      </div>
      {editNotice}
      <div className="flex-1 overflow-y-auto">
        {sources.map((source, index) => {
          const expanded = expandedSources[index] ?? true
          const selected = selectedSource === index
          return (
            <div key={source.ref || source.name || `source-${selectedSource}`}>
              <div
                className={cn(
                  'flex min-h-11 items-center gap-1 px-2 text-xs md:pointer-fine:min-h-0 md:pointer-fine:py-1.5',
                  'hover:bg-list-hover-background',
                  selected &&
                    'bg-list-active-selection-background text-list-active-selection-foreground',
                )}
              >
                <button
                  type="button"
                  className="flex min-h-11 min-w-0 flex-1 items-center gap-1 text-left md:pointer-fine:min-h-0"
                  onClick={() => {
                    onSelectSource(index)
                    toggleExpanded(index)
                  }}
                >
                  {expanded ? (
                    <LuChevronDown className="size-3 shrink-0" />
                  ) : (
                    <LuChevronRight className="size-3 shrink-0" />
                  )}
                  <LuFolder className="size-3 shrink-0" />
                  <span className="truncate">
                    {source.name || `Source ${index + 1}`}
                  </span>
                </button>
                <span className="flex shrink-0 items-center gap-0.5">
                  <button
                    type="button"
                    className="hover:bg-list-hover-background flex size-11 items-center justify-center rounded md:pointer-fine:size-auto md:pointer-fine:p-0.5"
                    title="Move source up"
                    disabled={sending || index === 0}
                    onClick={(e) => {
                      e.stopPropagation()
                      onMoveSource?.(index, -1)
                    }}
                  >
                    <LuArrowUp className="size-3" />
                  </button>
                  <button
                    type="button"
                    className="hover:bg-list-hover-background flex size-11 items-center justify-center rounded md:pointer-fine:size-auto md:pointer-fine:p-0.5"
                    title="Move source down"
                    disabled={sending || index === sources.length - 1}
                    onClick={(e) => {
                      e.stopPropagation()
                      onMoveSource?.(index, 1)
                    }}
                  >
                    <LuArrowDown className="size-3" />
                  </button>
                  <button
                    type="button"
                    className="hover:bg-list-hover-background flex size-11 items-center justify-center rounded md:pointer-fine:size-auto md:pointer-fine:p-0.5"
                    title="Remove source"
                    disabled={sending}
                    onClick={(e) => {
                      e.stopPropagation()
                      onRemoveSource?.(index)
                    }}
                  >
                    <LuTrash2 className="size-3" />
                  </button>
                </span>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

export default NotebookSidebar

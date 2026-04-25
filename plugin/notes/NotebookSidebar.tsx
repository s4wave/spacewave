import { useCallback } from 'react'

import type { NotebookSource } from './proto/notebook.pb.js'
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
  namespace: StateNamespace
}

// NotebookSidebar shows the source list from the Notebook.
function NotebookSidebar({
  sources,
  selectedSource,
  onSelectSource,
  onAddSource,
  onRemoveSource,
  onMoveSource,
  namespace,
}: NotebookSidebarProps) {
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

  if (sources.length === 0) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center p-4 text-center text-xs">
        No sources configured
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col overflow-y-auto">
      <div className="text-foreground-alt border-b border-border px-3 py-2 text-xs font-medium uppercase tracking-wide">
        <div className="flex items-center justify-between gap-2">
          <span>Sources</span>
          <button
            type="button"
            className="hover:bg-list-hover-background text-foreground-alt hover:text-foreground rounded p-1"
            title="Add source"
            onClick={onAddSource}
          >
            <LuPlus className="h-3 w-3" />
          </button>
        </div>
      </div>
      <div className="flex-1 overflow-y-auto">
        {sources.map((source, index) => {
          const expanded = expandedSources[index] ?? true
          const selected = selectedSource === index
          return (
            <div key={index}>
              <div
                className={cn(
                  'flex items-center gap-1 px-2 py-1.5 text-xs',
                  'hover:bg-list-hover-background',
                  selected &&
                    'bg-list-active-selection-background text-list-active-selection-foreground',
                )}
              >
                <button
                  type="button"
                  className="flex min-w-0 flex-1 items-center gap-1 text-left"
                  onClick={() => {
                    onSelectSource(index)
                    toggleExpanded(index)
                  }}
                >
                  {expanded ?
                    <LuChevronDown className="h-3 w-3 shrink-0" />
                  : <LuChevronRight className="h-3 w-3 shrink-0" />}
                  <LuFolder className="h-3 w-3 shrink-0" />
                  <span className="truncate">
                    {source.name || `Source ${index + 1}`}
                  </span>
                </button>
                <span className="flex shrink-0 items-center gap-0.5">
                  <button
                    type="button"
                    className="hover:bg-list-hover-background rounded p-0.5"
                    title="Move source up"
                    disabled={index === 0}
                    onClick={(e) => {
                      e.stopPropagation()
                      onMoveSource?.(index, -1)
                    }}
                  >
                    <LuArrowUp className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    className="hover:bg-list-hover-background rounded p-0.5"
                    title="Move source down"
                    disabled={index === sources.length - 1}
                    onClick={(e) => {
                      e.stopPropagation()
                      onMoveSource?.(index, 1)
                    }}
                  >
                    <LuArrowDown className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    className="hover:bg-list-hover-background rounded p-0.5"
                    title="Remove source"
                    onClick={(e) => {
                      e.stopPropagation()
                      onRemoveSource?.(index)
                    }}
                  >
                    <LuTrash2 className="h-3 w-3" />
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

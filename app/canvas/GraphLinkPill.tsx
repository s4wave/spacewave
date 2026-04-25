import { LuEyeOff, LuLocateFixed, LuPlus, LuTrash2 } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import type { EphemeralEdge } from './types.js'

interface GraphLinkPillProps {
  edge: EphemeralEdge
  loaded: boolean
  onPrimary: () => void
  onHide?: () => void
  onDelete?: () => void
}

// GraphLinkPill renders the inline Canvas graph-link action pill.
export function GraphLinkPill({
  edge,
  loaded,
  onPrimary,
  onHide,
  onDelete,
}: GraphLinkPillProps) {
  const capped =
    (edge.direction === 'out' && edge.outgoingTruncated) ||
    (edge.direction === 'in' && edge.incomingTruncated)

  return (
    <div className="flex h-[26px] max-w-64 items-center">
      <button
        className={cn(
          'bg-background-card/50 text-foreground border-foreground/10 hover:border-foreground/20 flex h-[26px] min-w-0 items-center gap-1 rounded-l-md border border-r-0 px-1.5 py-0.5 text-[0.6rem] shadow-lg backdrop-blur-sm transition-colors',
        )}
        onClick={onPrimary}
        title={`${loaded ? 'Focus' : 'Load'} ${edge.linkedObjectLabel}`}
      >
        <span className="text-brand/60 font-medium">{edge.predicate}</span>
        <span className="text-foreground-alt/30">/</span>
        <span className="max-w-24 truncate font-medium">
          {edge.linkedObjectLabel}
        </span>
        {edge.linkedObjectTypeLabel && (
          <span className="text-foreground-alt/50 max-w-20 truncate">
            {edge.linkedObjectTypeLabel}
          </span>
        )}
        {edge.protected && (
          <span className="border-foreground/8 bg-foreground/5 text-foreground-alt/50 rounded px-1 py-0.5">
            protected
          </span>
        )}
        {capped && (
          <span className="border-warning/20 bg-warning/10 text-warning rounded px-1 py-0.5">
            capped
          </span>
        )}
        {edge.hiddenCount > 0 && (
          <span className="border-foreground/8 bg-foreground/5 text-foreground-alt/50 rounded px-1 py-0.5">
            hidden {edge.hiddenCount}
          </span>
        )}
        <span className="ml-auto flex items-center gap-1">
          {loaded ?
            <LuLocateFixed className="h-3 w-3" />
          : <LuPlus className="h-3 w-3" />}
          {loaded ? 'Focus' : 'Load'}
        </span>
      </button>
      {edge.hideable && onHide && (
        <button
          className={cn(
            'bg-background-card/50 text-foreground border-foreground/10 hover:border-foreground/20 flex h-[26px] items-center border px-1 py-0.5 shadow-lg backdrop-blur-sm transition-colors',
            edge.userRemovable && onDelete ? 'border-r-0' : 'rounded-r-md',
          )}
          onClick={onHide}
          title={`Hide ${edge.predicate} link`}
        >
          <LuEyeOff className="h-3 w-3" />
        </button>
      )}
      {edge.userRemovable && onDelete && (
        <button
          className="bg-background-card/50 text-destructive border-foreground/10 hover:border-destructive/20 hover:bg-destructive/10 flex h-[26px] items-center rounded-r-md border px-1 py-0.5 shadow-lg backdrop-blur-sm transition-colors"
          onClick={onDelete}
          title={`Delete ${edge.predicate} link`}
        >
          <LuTrash2 className="h-3 w-3" />
        </button>
      )}
    </div>
  )
}

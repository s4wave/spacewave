import type { ReactNode } from 'react'

import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { LuCircleAlert } from 'react-icons/lu'

import type { EntityRow, KeyedSet } from '@s4wave/sdk/world/entity-set.js'
import {
  entitySetFailures,
  entitySetRows,
} from '@s4wave/web/hooks/useEntitySet.js'
import {
  resourceStatus,
  ViewerStatus,
} from '@s4wave/web/object/ViewerStatus.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

// ForgeSetRow is one member of a set with its decoded body.
export interface ForgeSetRow<T> {
  objectKey: string
  value: T
}

interface ForgeSetListProps<T> {
  set: Resource<KeyedSet<EntityRow<T>>>
  // what names the set in a failure, such as "the tasks".
  what: string
  icon: ReactNode
  loadingLabel: string
  emptyLabel: string
  // renderRow draws one row; it should return a memoized row keyed by
  // objectKey, so a change to one member draws only that row.
  renderRow: (row: ForgeSetRow<T>) => ReactNode
  // compare orders the rows; without it they keep the listing order.
  compare?: (a: ForgeSetRow<T>, b: ForgeSetRow<T>) => number
}

// ForgeSetList draws a followed set of Forge objects. Before the first page
// it shows the loading line, or the failure with Retry. Once rows arrived it
// keeps them through every later read and failure: a failure draws a notice
// with Retry above them, and while later pages of the first read load, a
// line under them says how many are still loading. A member whose body
// could not be decoded draws a row that says so, after the others.
export function ForgeSetList<T>({
  set,
  what,
  icon,
  loadingLabel,
  emptyLabel,
  renderRow,
  compare,
}: ForgeSetListProps<T>) {
  const value = resourceContent(set)
  const status = resourceStatus(what, set)
  const listed = entitySetRows(value)
  const rows = compare ? listed.toSorted(compare) : listed
  const undecoded = entitySetFailures(value)
  const failed = status?.state === 'failed' ? status : null

  if (
    !value ||
    (rows.length === 0 && undecoded.length === 0 && !value.complete)
  ) {
    if (failed) return <ViewerStatus {...failed} />
    return (
      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <div className="text-foreground-alt/40 flex items-center gap-2 p-1 text-xs">
          <LoadingInline label={loadingLabel} size="sm" tone="subtle" />
        </div>
      </div>
    )
  }

  const loadingMore = value.complete
    ? 0
    : value.keys.length - rows.length - undecoded.length
  return (
    <div className="space-y-2">
      {failed && <ViewerStatus {...failed} />}
      {rows.length === 0 && undecoded.length === 0 ? (
        <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
          <div className="text-foreground-alt/40 flex items-center gap-2 p-1 text-xs">
            <span className="shrink-0">{icon}</span>
            <span>{emptyLabel}</span>
          </div>
        </div>
      ) : (
        rows.map(renderRow)
      )}
      {undecoded.map(({ objectKey, error }) => (
        <div
          key={objectKey}
          className="border-destructive/20 bg-background-card/30 flex items-center gap-2 rounded-lg border px-3.5 py-2.5"
        >
          <LuCircleAlert className="text-destructive size-3 shrink-0" />
          <div className="min-w-0">
            <div className="text-foreground truncate text-sm">
              Could not read {objectKey}
            </div>
            <div className="text-foreground-alt/50 truncate text-xs">
              {error}
            </div>
          </div>
        </div>
      ))}
      {loadingMore > 0 && (
        <div className="text-foreground-alt/40 flex items-center gap-2 p-1 text-xs">
          <LoadingInline
            label={`Loading ${loadingMore} more`}
            size="sm"
            tone="subtle"
          />
        </div>
      )}
    </div>
  )
}

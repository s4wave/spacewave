import type { ReactNode } from 'react'
import { LuCircleAlert, LuInbox } from 'react-icons/lu'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { FactList, factGrid } from '@s4wave/web/ui/FactList.js'

/**
 * ViewerStatusProps is what a viewer shows before, instead of or above its
 * object:
 *
 * - waiting: nothing has arrived yet; what names the record, such as "the
 *   notebook".
 * - failed: the read failed; message says what went wrong. retry is set only
 *   when reading again can succeed, and the notice offers it. The notice is
 *   one row, so a viewer whose record arrived earlier draws it above the
 *   record rather than in its place.
 * - empty: the record arrived and holds nothing to show; message says so and,
 *   where one exists, names the press that fills it.
 *
 * ViewerStatus.tla checks that each press shows where it can succeed and that
 * a record that arrived stays drawn through a failure or a re-read.
 */
export type ViewerStatusProps =
  | { state: 'waiting'; what: string; rows?: number }
  | {
      state: 'failed'
      message: string
      retry?: () => void
      retryLabel?: string
    }
  | { state: 'empty'; message: ReactNode; icon?: ReactNode }

/**
 * resourceStatus returns the status of the resources a viewer reads: failed
 * with the failed resource's retry, waiting while one is read with nothing
 * arrived yet, or null. A resource read again keeps its last value while the
 * read runs, so it does not wait. A failed resource holds no value; when other
 * resources hold data, the caller draws the failure above that data. what
 * names the record, such as "the notebook".
 */
export function resourceStatus(
  what: string,
  ...resources: Resource<unknown>[]
): ViewerStatusProps | null {
  const failed = resources.find((r) => r.error)
  if (failed?.error) {
    return {
      state: 'failed',
      message: `Could not read ${what}: ${failed.error.message}`,
      retry: failed.retry,
    }
  }
  if (resources.some((r) => r.loading && r.value == null)) {
    return { state: 'waiting', what }
  }
  return null
}

// padClass insets a row from the pane's edge as the bar is.
const padClass =
  'ps-viewer-pad pe-viewer-pad @pane-roomy:ps-viewer-pad-roomy @pane-roomy:pe-viewer-pad-roomy'

// rowClass is one row of the viewer body: a fact row tall, padded as the bar.
const rowClass = cn(
  'min-h-viewer-fact-row flex items-center gap-2 text-xs',
  padClass,
)

// markClass is the column a row's icon sits in, the bar's icon column, so the
// row's text starts under the bar's title.
const markClass = 'flex shrink-0 [&_svg]:size-3.5'

const skeletonClass =
  'bg-foreground/10 h-2 rounded-xs motion-safe:animate-pulse'

// labelWidths and valueWidths vary the skeleton rows so they read as facts.
const labelWidths = ['w-12', 'w-16', 'w-20']
const valueWidths = ['w-2/5', 'w-3/5', 'w-1/2', 'w-4/5', 'w-2/3']

/**
 * ViewerStatus draws a viewer's waiting, failed or empty state in the viewer
 * pane, under the viewer's bar. It sizes by its containers, not the window:
 * rows keep the fact row's height and the pane's padding, and the skeleton
 * rows sit in a FactList, so their label column is the one the viewer's facts
 * take at the same width. Waiting draws skeleton rows under a line naming
 * what is read; failed is a notice row with Retry when a read can run again;
 * empty is one muted row.
 */
export function ViewerStatus(props: ViewerStatusProps) {
  if (props.state === 'waiting') {
    return (
      <div
        role="status"
        className="text-foreground-alt flex h-full min-h-0 w-full flex-col overflow-hidden"
      >
        <p className={rowClass}>Loading {props.what}</p>
        <div className={padClass}>
          <FactList>
            {Array.from({ length: props.rows ?? 5 }, (_, i) => (
              <div
                key={i}
                aria-hidden
                className={cn(
                  factGrid,
                  'min-h-viewer-fact-row border-foreground/5 items-center border-b',
                )}
              >
                <span className="flex">
                  <span
                    className={cn(
                      skeletonClass,
                      labelWidths[i % labelWidths.length],
                    )}
                  />
                </span>
                <span className="flex min-w-0">
                  <span
                    className={cn(
                      skeletonClass,
                      valueWidths[i % valueWidths.length],
                    )}
                  />
                </span>
              </div>
            ))}
          </FactList>
        </div>
      </div>
    )
  }

  if (props.state === 'failed') {
    return (
      <div
        role="alert"
        className={cn(
          rowClass,
          'border-foreground/8 bg-destructive/6 text-foreground shrink-0 border-b py-1',
        )}
      >
        <span aria-hidden className={cn(markClass, 'text-destructive')}>
          <LuCircleAlert />
        </span>
        <span className="max-w-prose min-w-0 flex-1 leading-snug break-words">
          {props.message}
        </span>
        {props.retry ? (
          <Button
            variant="outline"
            size="xs"
            className="shrink-0"
            onClick={props.retry}
          >
            {props.retryLabel ?? 'Retry'}
          </Button>
        ) : null}
      </div>
    )
  }

  return (
    <p className={cn(rowClass, 'text-foreground-alt/70 shrink-0')}>
      <span aria-hidden className={markClass}>
        {props.icon ?? <LuInbox />}
      </span>
      <span className="max-w-prose min-w-0 flex-1 leading-snug">
        {props.message}
      </span>
    </p>
  )
}

import type { ReactNode } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { ViewerStage } from '@s4wave/web/object/ViewerFloat.js'
import {
  ViewerStatus,
  resourceStatus,
  type ViewerStatusProps,
} from '@s4wave/web/object/ViewerStatus.js'

// sendFailedMessage is the notice for an edit the canvas did not accept.
export const sendFailedMessage =
  'Could not save the last change to this canvas, so it was undone.'

// CanvasViewState is what the Canvas viewer draws, derived from its canvas
// watch as CanvasViewer.tla models it: the watch's status in place of the
// stage until a canvas arrives, then the stage with at most a line while the
// watch reconnects and a notice for a failed edit.
export type CanvasViewState =
  | { body: 'status'; status: ViewerStatusProps }
  | {
      body: 'stage'
      notice?: ViewerStatusProps
      action?: string
      empty: boolean
    }

/**
 * canvasViewState derives the drawn state from the canvas watch, the failed
 * edit's message, and whether the canvas the person sees holds no node. The
 * watch reports a failure only with no canvas held: once one arrived it
 * reconnects with the canvas kept, which draws as a waiting line.
 */
export function canvasViewState(
  watch: Resource<unknown>,
  actionError: string | null,
  empty: boolean,
): CanvasViewState {
  if (watch.value == null) {
    const status = resourceStatus('the canvas', watch)
    return {
      body: 'status',
      status:
        status?.state === 'failed'
          ? status
          : { state: 'waiting', what: 'the canvas', rows: 0 },
    }
  }
  const notice: ViewerStatusProps | undefined = watch.loading
    ? { state: 'waiting', what: 'the canvas again', rows: 0 }
    : undefined
  return {
    body: 'stage',
    notice,
    action: actionError ?? undefined,
    empty,
  }
}

export interface CanvasViewProps {
  // view is the derived state to draw.
  view: CanvasViewState
  // onDismiss clears the failed edit's notice.
  onDismiss: () => void
  // children is the canvas, drawn on the stage.
  children: ReactNode
}

// noticesClass floats the notices over the stage's top edge, clear of the
// canvas's tool palette on the left and its minimap at the bottom right, on a
// card that keeps them readable over any node. Drawn over the stage, a notice
// never changes the stage's box, so nodes stay put while one comes and goes.
const noticesClass =
  'border-foreground/10 bg-background/94 absolute top-2 right-2 left-14 z-20 flex flex-col overflow-hidden rounded-md border shadow-md [&>*:last-child]:border-b-0'

// CanvasView draws the canvas viewer's state: the status in place of the
// stage, or the stage with its notices floating over it and a hint over an
// empty canvas.
export function CanvasView({ view, onDismiss, children }: CanvasViewProps) {
  if (view.body === 'status') return <ViewerStatus {...view.status} />
  const notices = view.notice || view.action
  return (
    <ViewerStage media="still">
      {view.empty && (
        <p className="text-foreground-alt pointer-events-none absolute inset-0 z-10 flex items-center justify-center px-4 text-center text-xs">
          Empty canvas. Pick a tool on the left to add an object.
        </p>
      )}
      {children}
      {notices ? (
        <div data-testid="canvas-notices" className={noticesClass}>
          {view.notice ? <ViewerStatus {...view.notice} /> : null}
          {view.action ? (
            <ViewerStatus
              state="failed"
              message={view.action}
              retry={onDismiss}
              retryLabel="Dismiss"
            />
          ) : null}
        </div>
      ) : null}
    </ViewerStage>
  )
}

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { resourceContent, resourceOf } from '@aptre/bldr-sdk/hooks/index.js'
import { useParams } from '@s4wave/web/router/router.js'

import { Canvas } from '@s4wave/app/canvas/Canvas.js'
import type {
  CanvasCallbacks,
  CanvasNodeData,
  CanvasStateData,
} from '@s4wave/app/canvas/types.js'
import {
  CanvasView,
  canvasViewState,
  sendFailedMessage,
} from '@s4wave/app/canvas/viewer/CanvasView.js'

// textNode is one text card on the canvas.
function textNode(
  id: string,
  x: number,
  y: number,
  textContent: string,
): CanvasNodeData {
  return {
    id,
    x,
    y,
    width: 220,
    height: 120,
    zIndex: 0,
    type: 'text',
    textContent,
  }
}

// populated is a canvas with three cards and two edges between them.
const populated: CanvasStateData = {
  nodes: new Map(
    [
      textNode('plan', 40, 40, '# Launch plan\nShip the gallery and canvas.'),
      textNode('risks', 320, 60, 'Risks: the watch drops on a flaky link.'),
      textNode('notes', 180, 240, 'Notes from the walk-through.'),
    ].map((node) => [node.id, node]),
  ),
  edges: [
    {
      id: 'plan-risks',
      sourceNodeId: 'plan',
      targetNodeId: 'risks',
      style: 'bezier',
    },
    {
      id: 'plan-notes',
      sourceNodeId: 'plan',
      targetNodeId: 'notes',
      style: 'straight',
    },
  ],
  hiddenGraphLinks: [],
  layoutMetadata: new Map(),
}

// emptyCanvas is a canvas with no node.
const emptyCanvas: CanvasStateData = {
  nodes: new Map(),
  edges: [],
  hiddenGraphLinks: [],
  layoutMetadata: new Map(),
}

// noop stands in for a Retry or Dismiss press.
const noop = () => {}

// callbacks drop every edit, since the fixture holds no handle.
const callbacks: CanvasCallbacks = {}

interface CanvasFixture {
  // watch is the canvas watch's resource; its value stands for the canvas.
  watch: Resource<CanvasStateData>
  // actionError is the failed edit's message.
  actionError: string | null
}

// watch is the canvas watch in one state.
function watch(
  value: CanvasStateData | null,
  {
    loading = false,
    error = null as Error | null,
    last = null as CanvasStateData | null,
  } = {},
): Resource<CanvasStateData> {
  return resourceOf({ value, last, loading, error, retry: noop })
}

// fixtures are the states of CanvasViewer.tla, as the watch reports them.
const fixtures: Record<string, CanvasFixture> = {
  populated: { watch: watch(populated), actionError: null },
  waiting: { watch: watch(null, { loading: true }), actionError: null },
  failed: {
    watch: watch(null, { error: new Error('stream closed') }),
    actionError: null,
  },
  empty: { watch: watch(emptyCanvas), actionError: null },
  again: { watch: watch(populated, { loading: true }), actionError: null },
  'failed-kept': {
    watch: watch(null, { error: new Error('stream closed'), last: populated }),
    actionError: null,
  },
  'save-failed': {
    watch: watch(populated),
    actionError: sendFailedMessage,
  },
}

/**
 * CanvasViewerDebug renders the Canvas viewer in one fixture state, with the
 * viewport as the pane, for the viewer sweep: populated, waiting, failed
 * (the first read or the handle failed, with Retry), empty (with the hint),
 * again (the canvas while the watch reconnects), failed-kept (the last
 * canvas under the watch's failure past its backoff, with Retry) and save-failed (the canvas under the failed edit's
 * notice). An unknown or missing state shows populated. The view derives
 * from the watch's resource as the viewer's does.
 */
export function CanvasViewerDebug() {
  // Read the fixture state from the route.
  const { state = 'populated' } = useParams()
  const fixture = fixtures[state] ?? fixtures.populated
  const canvas = resourceContent(fixture.watch) ?? emptyCanvas
  const view = canvasViewState(
    fixture.watch,
    fixture.actionError,
    canvas.nodes.size === 0,
  )

  return (
    <div className="bg-background-primary viewer-pane h-full w-full">
      <CanvasView view={view} onDismiss={noop}>
        <Canvas
          state={canvas}
          callbacks={callbacks}
          className="h-full w-full"
        />
      </CanvasView>
    </div>
  )
}

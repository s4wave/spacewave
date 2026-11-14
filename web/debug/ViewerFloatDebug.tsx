import { useEffect, useRef } from 'react'
import {
  LuChevronLeft,
  LuChevronRight,
  LuDownload,
  LuScan,
} from 'react-icons/lu'

import { useParams } from '@s4wave/web/router/router.js'
import {
  type ViewerMedia,
  ViewerFloat,
  ViewerFloatLink,
  ViewerFloatPress,
  ViewerStage,
} from '@s4wave/web/object/ViewerFloat.js'

// fixtureMedia maps each fixture state to what the stage's content does.
const fixtureMedia: Record<string, ViewerMedia> = {
  shown: 'still',
  hidden: 'moving',
  pointer: 'moving',
  focused: 'moving',
  failed: 'failed',
}

// fixtureIdleMs is each state's idle window: the pointer state holds however
// long the sweep takes, and the hidden state leaves at once.
const fixtureIdleMs: Record<string, number> = {
  hidden: 0,
  pointer: 24 * 60 * 60 * 1000,
}

/**
 * ViewerFloatDebug draws floating viewer controls over bright, dark and busy
 * content in one state, with the viewport as the pane, for the viewer sweep:
 * shown over still content, hidden over moving content nobody touched, held
 * by the pointer, held by focus, and over failed media. An unknown or missing
 * state shows the floats over still content.
 */
export function ViewerFloatDebug() {
  // Read the fixture state from the route.
  const { state = 'shown' } = useParams()
  const media = fixtureMedia[state] ?? 'still'
  const ref = useRef<HTMLDivElement | null>(null)

  // Put the pointer over the stage, or focus in the floats, as the state asks.
  useEffect(() => {
    const root = ref.current
    if (!root) return
    if (state === 'pointer') {
      root.firstElementChild?.dispatchEvent(
        new PointerEvent('pointermove', { bubbles: true }),
      )
    }
    if (state === 'focused') {
      root.querySelector<HTMLElement>('[role=toolbar] button')?.focus()
    }
  }, [state])

  return (
    <div ref={ref} className="viewer-pane flex h-full w-full bg-black">
      <ViewerStage media={media} idleMs={fixtureIdleMs[state]}>
        <div
          aria-hidden
          className="flex-1 bg-linear-to-br from-white from-30% via-yellow-400 to-black"
        />
        <ViewerFloat at="tr" label="Image">
          <ViewerFloatPress label="Actual size" icon={<LuScan aria-hidden />} />
          <ViewerFloatLink
            label="Download"
            icon={<LuDownload aria-hidden />}
            essential
            href="#"
          />
        </ViewerFloat>
        <ViewerFloat at="bc" label="Pages">
          <ViewerFloatPress
            label="Previous page"
            icon={<LuChevronLeft aria-hidden />}
            essential
            disabled
          />
          <span className="text-foreground-alt px-1 text-xs tabular-nums">
            Page 1 of 12
          </span>
          <ViewerFloatPress
            label="Next page"
            icon={<LuChevronRight aria-hidden />}
            essential
          />
        </ViewerFloat>
      </ViewerStage>
    </div>
  )
}

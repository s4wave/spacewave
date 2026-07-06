import type { ReactNode } from 'react'
import {
  LuArrowDownRight,
  LuArrowLeft,
  LuArrowUp,
  LuSparkles,
} from 'react-icons/lu'

import { Button } from '@s4wave/web/ui/button.js'
import {
  IntroWizardRegion,
  type IntroWizardCallout,
} from '@s4wave/sdk/world/wizard/wizard.pb.js'

export interface IntroWizardOverlayProps {
  headline: string
  subhead: string
  finishLabel: string
  callouts: IntroWizardCallout[]
  finishing: boolean
  onFinish: () => void
  onSkip: () => void
}

// IntroWizardOverlay draws the new-user introduction around an object frame:
// labeled callouts pointing at regions of the UI plus a control panel that ends
// the introduction. The root is click-through so the live UI stays usable while
// the introduction is shown; only the control panel captures pointer events.
export function IntroWizardOverlay({
  headline,
  subhead,
  finishLabel,
  callouts,
  finishing,
  onFinish,
  onSkip,
}: IntroWizardOverlayProps) {
  return (
    <div className="pointer-events-none absolute inset-0 z-20 overflow-hidden">
      {/* The dim veil draws focus to the introduction but leaves the bottom-right
          upload-indicator anchor lit: the "Upload progress" callout teaches the
          user to watch that live element, so it must not read as disabled. The
          two panels tile the screen around a w-72 h-40 lit corner window. */}
      <div
        data-testid="intro-scrim"
        className="bg-background/60 absolute inset-x-0 top-0 bottom-40"
      />
      <div
        data-testid="intro-scrim"
        className="bg-background/60 absolute right-72 bottom-0 left-0 h-40"
      />
      {callouts.map((callout, index) => (
        <IntroCallout key={callout.title || index} callout={callout} />
      ))}
      <div className="border-brand/30 bg-background-card/95 pointer-events-auto absolute bottom-8 left-1/2 flex max-w-sm -translate-x-1/2 flex-col gap-2 rounded-xl border p-4 shadow-lg backdrop-blur">
        <div className="flex items-center gap-2">
          <LuSparkles className="text-brand size-4 shrink-0" />
          <h2 className="text-foreground text-sm font-semibold">{headline}</h2>
        </div>
        {subhead && (
          <p className="text-foreground-alt/70 text-xs leading-relaxed">
            {subhead}
          </p>
        )}
        <div className="mt-1 flex items-center justify-end gap-2">
          <Button
            size="sm"
            variant="ghost"
            onClick={onSkip}
            disabled={finishing}
            className="text-foreground-alt/70 hover:text-foreground h-8 rounded-md px-3 text-xs font-medium"
          >
            Skip
          </Button>
          <Button
            size="sm"
            onClick={onFinish}
            disabled={finishing}
            className="border-brand/60 bg-brand/25 hover:border-brand/80 hover:bg-brand/35 text-foreground h-8 rounded-md border px-3 text-xs font-medium"
          >
            {finishing ? 'Opening...' : finishLabel}
          </Button>
        </div>
      </div>
    </div>
  )
}

interface CalloutLayout {
  container: string
  arrow: ReactNode
  arrowPosition: 'before' | 'after' | 'none'
}

function IntroCallout({ callout }: { callout: IntroWizardCallout }) {
  const layout = calloutLayout(callout.region)
  return (
    <div className={layout.container}>
      {layout.arrowPosition === 'before' && layout.arrow}
      <div className="border-foreground/10 bg-background-card/95 max-w-[13rem] rounded-lg border px-3 py-2 shadow-md backdrop-blur">
        <div className="text-foreground text-xs font-semibold">
          {callout.title}
        </div>
        <p className="text-foreground-alt/70 mt-0.5 text-[0.7rem] leading-relaxed">
          {callout.detail}
        </p>
      </div>
      {layout.arrowPosition === 'after' && layout.arrow}
    </div>
  )
}

// calloutLayout maps an intro region to the callout card placement and the
// arrow glyph that points at the UI part the label describes.
function calloutLayout(region: IntroWizardRegion | undefined): CalloutLayout {
  const arrowClass = 'text-brand size-6 shrink-0'
  switch (region) {
    case IntroWizardRegion.TOP:
      return {
        container:
          'absolute left-1/2 top-12 flex -translate-x-1/2 flex-col items-center gap-1',
        arrow: <LuArrowUp className={arrowClass} />,
        arrowPosition: 'before',
      }
    case IntroWizardRegion.LEFT:
      return {
        container:
          'absolute left-8 top-[42%] flex -translate-y-1/2 flex-row items-center gap-1',
        arrow: <LuArrowLeft className={arrowClass} />,
        arrowPosition: 'before',
      }
    case IntroWizardRegion.BOTTOM_RIGHT:
      return {
        container: 'absolute bottom-24 right-12 flex flex-col items-end gap-1',
        arrow: <LuArrowDownRight className={arrowClass} />,
        arrowPosition: 'after',
      }
    default:
      return {
        container:
          'absolute left-1/2 top-[40%] flex -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-1',
        arrow: null,
        arrowPosition: 'none',
      }
  }
}

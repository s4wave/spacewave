import { useState, type ReactNode } from 'react'
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

// IntroWizardOverlay draws the new-user introduction around an object frame as
// an ordered tour: one callout points at one region at a time while the control
// panel narrates the tour and advances through the steps. Presenting a single
// callout per step avoids stacking every pointer at once. The root is
// click-through so the live UI stays usable while the introduction is shown;
// only the control panel captures pointer events. The last step's primary
// action ends the introduction.
export function IntroWizardOverlay({
  headline,
  subhead,
  finishLabel,
  callouts,
  finishing,
  onFinish,
  onSkip,
}: IntroWizardOverlayProps) {
  const [step, setStep] = useState(0)
  const stepCount = callouts.length
  // Clamp so a shrinking callout list (or an empty one) can never strand the
  // tour past its last step with no primary action to finish.
  const activeStep = stepCount === 0 ? 0 : Math.min(step, stepCount - 1)
  const currentCallout = callouts[activeStep]
  const isLastStep = activeStep >= stepCount - 1

  return (
    <div className="pointer-events-none absolute inset-0 z-20">
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
      {currentCallout && <IntroCallout callout={currentCallout} />}
      <div className="border-brand/30 bg-background-card/95 pointer-events-auto absolute bottom-8 left-1/2 z-40 flex max-w-sm -translate-x-1/2 flex-col gap-2 rounded-xl border p-4 shadow-lg backdrop-blur">
        <div className="flex items-center gap-2">
          <LuSparkles className="text-brand size-4 shrink-0" />
          <h2 className="text-foreground text-sm font-semibold">{headline}</h2>
        </div>
        {subhead && (
          <p className="text-foreground-alt/70 text-xs leading-relaxed">
            {subhead}
          </p>
        )}
        {stepCount > 1 && (
          <div
            className="mt-1 flex items-center gap-1.5"
            aria-label={`Step ${activeStep + 1} of ${stepCount}`}
          >
            {callouts.map((callout, index) => (
              <span
                key={callout.title || index}
                className={
                  index === activeStep
                    ? 'bg-brand h-1.5 w-4 rounded-full transition-all'
                    : 'bg-foreground/20 h-1.5 w-1.5 rounded-full transition-all'
                }
              />
            ))}
          </div>
        )}
        <div className="mt-1 flex items-center justify-between gap-2">
          <Button
            size="sm"
            variant="ghost"
            onClick={onSkip}
            disabled={finishing}
            className="text-foreground-alt/70 hover:text-foreground h-8 rounded-md px-3 text-xs font-medium"
          >
            Skip
          </Button>
          <div className="flex items-center gap-2">
            {activeStep > 0 && (
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setStep(activeStep - 1)}
                disabled={finishing}
                className="text-foreground-alt/70 hover:text-foreground h-8 rounded-md px-3 text-xs font-medium"
              >
                Back
              </Button>
            )}
            <Button
              size="sm"
              onClick={() =>
                isLastStep ? onFinish() : setStep(activeStep + 1)
              }
              disabled={finishing}
              className="border-brand/60 bg-brand/25 hover:border-brand/80 hover:bg-brand/35 text-foreground h-8 rounded-md border px-3 text-xs font-medium"
            >
              {isLastStep ? (finishing ? 'Opening...' : finishLabel) : 'Next'}
            </Button>
          </div>
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
// arrow glyph that points at the UI part the label describes. The z-30 keeps
// the active callout above the scrim and any target-frame chrome it overlaps so
// its label is never clipped behind a card in the introduced object.
function calloutLayout(region: IntroWizardRegion | undefined): CalloutLayout {
  const arrowClass = 'text-brand size-6 shrink-0'
  switch (region) {
    case IntroWizardRegion.TOP:
      return {
        container:
          'absolute left-1/2 top-12 z-30 flex -translate-x-1/2 flex-col items-center gap-1',
        arrow: <LuArrowUp className={arrowClass} />,
        arrowPosition: 'before',
      }
    case IntroWizardRegion.LEFT:
      return {
        container:
          'absolute left-8 top-[42%] z-30 flex -translate-y-1/2 flex-row items-center gap-1',
        arrow: <LuArrowLeft className={arrowClass} />,
        arrowPosition: 'before',
      }
    case IntroWizardRegion.BOTTOM_RIGHT:
      return {
        container:
          'absolute right-12 bottom-24 z-30 flex flex-col items-end gap-1',
        arrow: <LuArrowDownRight className={arrowClass} />,
        arrowPosition: 'after',
      }
    default:
      return {
        container:
          'absolute left-1/2 top-[40%] z-30 flex -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-1',
        arrow: null,
        arrowPosition: 'none',
      }
  }
}

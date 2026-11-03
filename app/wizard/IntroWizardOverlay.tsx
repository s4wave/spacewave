import { useState, type ReactNode } from 'react'
import {
  LuArrowDownRight,
  LuArrowLeft,
  LuArrowUp,
  LuSparkles,
} from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import {
  IntroWizardRegion,
  type IntroWizardCallout,
} from '@s4wave/sdk/world/wizard/wizard.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    gap: themeLayout['--spacing'],
    alignItems: 'flex-end',
    display: 'flex',
    flexDirection: 'column',
    position: 'absolute',
    zIndex: 30,
    right: `calc(${themeLayout['--spacing']} * 4)`,
    top: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule2: {
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'row',
    position: 'absolute',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    zIndex: 30,
    left: `calc(${themeLayout['--spacing']} * 8)`,
    top: '42%',
  },
  rule3: {
    gap: themeLayout['--spacing'],
    alignItems: 'flex-end',
    display: 'flex',
    flexDirection: 'column',
    position: 'absolute',
    zIndex: 30,
    bottom: `calc(${themeLayout['--spacing']} * 24)`,
    right: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule4: {
    '--tw-translate-x': 'calc(calc(1 / 2 * 100%) * -1)',
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    position: 'absolute',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    zIndex: 30,
    left: '50%',
    top: '40%',
  },
})

const stylexStyles2 = stylex.create({
  rule1: { inset: 0, pointerEvents: 'none', position: 'absolute', zIndex: 20 },
  rule2: {
    insetInline: 0,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 60%, transparent)`,
    position: 'absolute',
    bottom: `calc(${themeLayout['--spacing']} * 40)`,
    top: 0,
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 60%, transparent)`,
    position: 'absolute',
    bottom: 0,
    height: `calc(${themeLayout['--spacing']} * 40)`,
    left: 0,
    right: `calc(${themeLayout['--spacing']} * 72)`,
  },
  rule4: {
    '--tw-backdrop-blur': 'blur(8px)',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    '--tw-translate-x': 'calc(calc(1 / 2 * 100%) * -1)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 95%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    display: 'flex',
    flexDirection: 'column',
    pointerEvents: 'auto',
    position: 'absolute',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    zIndex: 40,
    bottom: `calc(${themeLayout['--spacing']} * 8)`,
    left: '50%',
    maxWidth: 'var(--container-sm)',
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule6: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule7: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
    marginTop: themeLayout['--spacing'],
  },
  rule9: {
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
    transitionProperty: 'width, background-color',
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule10: {
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    transitionProperty: 'width, background-color',
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule11: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginTop: themeLayout['--spacing'],
  },
  rule12: {
    '--tw-backdrop-blur': 'blur(8px)',
    '--tw-shadow':
      '0 4px 6px -1px var(--tw-shadow-color, #0000001a), 0 2px 4px -2px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 95%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    maxWidth: `calc(${themeLayout['--spacing']} * 52)`,
  },
  rule13: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule14: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: '.7rem',
    lineHeight: 'var(--leading-relaxed)',
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    marginRight: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule3: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
})

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
    <div sx={stylexStyles2.rule1}>
      {/* The dim veil draws focus to the introduction but leaves the bottom-right
          upload-indicator anchor lit: the "Upload progress" callout teaches the
          user to watch that live element, so it must not read as disabled. The
          two panels tile the screen around a w-72 h-40 lit corner window. */}
      <div data-testid="intro-scrim" sx={stylexStyles2.rule2} />
      <div data-testid="intro-scrim" sx={stylexStyles2.rule3} />
      {currentCallout && <IntroCallout callout={currentCallout} />}
      <div sx={stylexStyles2.rule4}>
        <div sx={stylexStyles2.rule5}>
          <LuSparkles sx={stylexStyles.rule1} />
          <h2 sx={stylexStyles2.rule6}>{headline}</h2>
        </div>
        {subhead && <p sx={stylexStyles2.rule7}>{subhead}</p>}
        {stepCount > 1 && (
          <div
            sx={stylexStyles2.rule8}
            aria-label={`Step ${activeStep + 1} of ${stepCount}`}
          >
            {callouts.map((callout, index) => (
              <span
                key={callout.title || index}
                sx={
                  index === activeStep
                    ? stylexStyles2.rule9
                    : stylexStyles2.rule10
                }
              />
            ))}
          </div>
        )}
        <div sx={stylexStyles2.rule11}>
          <Button
            onClick={onSkip}
            disabled={finishing}
            variant="muted"
            size="sm"
          >
            Skip
          </Button>
          <div sx={stylexStyles2.rule5}>
            {activeStep > 0 && (
              <Button
                onClick={() => setStep(activeStep - 1)}
                disabled={finishing}
                variant="muted"
                size="sm"
              >
                Back
              </Button>
            )}
            <Button
              onClick={() =>
                isLastStep ? onFinish() : setStep(activeStep + 1)
              }
              disabled={finishing}
              variant="brandSoft"
              size="sm"
            >
              {isLastStep ? (finishing ? 'Opening…' : finishLabel) : 'Next'}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

interface CalloutLayout {
  container: stylex.StyleXStyles
  arrow: ReactNode
  arrowPosition: 'before' | 'after' | 'none'
}

function IntroCallout({ callout }: { callout: IntroWizardCallout }) {
  const layout = calloutLayout(callout.region)
  return (
    <div sx={layout.container}>
      {layout.arrowPosition === 'before' && layout.arrow}
      <div sx={stylexStyles2.rule12}>
        <div sx={stylexStyles2.rule13}>{callout.title}</div>
        <p sx={stylexStyles2.rule14}>{callout.detail}</p>
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
  switch (region) {
    case IntroWizardRegion.TOP:
      return {
        container: stylexStyles3.rule1,
        arrow: <LuArrowUp sx={stylexStyles.rule2} />,
        arrowPosition: 'before',
      }
    case IntroWizardRegion.LEFT:
      return {
        container: stylexStyles3.rule2,
        arrow: <LuArrowLeft sx={stylexStyles.rule3} />,
        arrowPosition: 'before',
      }
    case IntroWizardRegion.BOTTOM_RIGHT:
      return {
        container: stylexStyles3.rule3,
        arrow: <LuArrowDownRight sx={stylexStyles.rule3} />,
        arrowPosition: 'after',
      }
    default:
      return {
        container: stylexStyles3.rule4,
        arrow: null,
        arrowPosition: 'none',
      }
  }
}

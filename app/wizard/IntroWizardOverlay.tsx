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
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 12)`,
    right: `calc(${themeLayout['--spacing']} * 4)`,
    zIndex: 30,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'flex-end',
    gap: themeLayout['--spacing'],
  },
  rule2: {
    position: 'absolute',
    top: '42%',
    left: `calc(${themeLayout['--spacing']} * 8)`,
    zIndex: 30,
    display: 'flex',
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    flexDirection: 'row',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule3: {
    position: 'absolute',
    right: `calc(${themeLayout['--spacing']} * 12)`,
    bottom: `calc(${themeLayout['--spacing']} * 24)`,
    zIndex: 30,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'flex-end',
    gap: themeLayout['--spacing'],
  },
  rule4: {
    position: 'absolute',
    top: '40%',
    left: '50%',
    zIndex: 30,
    display: 'flex',
    '--tw-translate-x': 'calc(calc(1 / 2 * 100%) * -1)',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    flexDirection: 'column',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: { pointerEvents: 'none', position: 'absolute', inset: 0, zIndex: 20 },
  rule2: {
    position: 'absolute',
    insetInline: 0,
    top: 0,
    bottom: `calc(${themeLayout['--spacing']} * 40)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 60%, transparent)`,
  },
  rule3: {
    position: 'absolute',
    right: `calc(${themeLayout['--spacing']} * 72)`,
    bottom: 0,
    left: 0,
    height: `calc(${themeLayout['--spacing']} * 40)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 60%, transparent)`,
  },
  rule4: {
    pointerEvents: 'auto',
    position: 'absolute',
    bottom: `calc(${themeLayout['--spacing']} * 8)`,
    left: '50%',
    zIndex: 40,
    display: 'flex',
    maxWidth: 'var(--container-sm)',
    '--tw-translate-x': 'calc(calc(1 / 2 * 100%) * -1)',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 95%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(8px)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule8: {
    marginTop: themeLayout['--spacing'],
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule9: {
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
    transitionProperty: 'width, background-color',
  },
  rule10: {
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    transitionProperty: 'width, background-color',
  },
  rule11: {
    marginTop: themeLayout['--spacing'],
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: {
    maxWidth: `calc(${themeLayout['--spacing']} * 52)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 95%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    '--tw-shadow':
      '0 4px 6px -1px var(--tw-shadow-color, #0000001a), 0 2px 4px -2px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(8px)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule13: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule14: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.7rem',
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-brand'],
  },
  rule2: {
    marginRight: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    flexShrink: 0,
    color: themeColors['--color-brand'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    flexShrink: 0,
    color: themeColors['--color-brand'],
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

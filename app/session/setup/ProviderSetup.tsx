import { useCallback } from 'react'
import { LuArrowRight, LuCloud, LuHardDrive } from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useLocalSessionOnboardingContext } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
})

// ProviderSetup renders the provider choice step of the setup wizard.
// Users select between Spacewave Cloud (paid, multi-device) and Local Only (free, browser-only).
export function ProviderSetup() {
  const navigate = useNavigate()
  const onboarding = useLocalSessionOnboardingContext()

  const handleLocal = useCallback(() => {
    onboarding.markProviderChoiceComplete()
    navigate({ path: '../' })
  }, [onboarding, navigate])

  const handleCloud = useCallback(() => {
    navigate({ path: '../../plan' })
  }, [navigate])

  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center gap-6 overflow-y-auto p-6 outline-none md:p-10">
      <div className="relative z-10 flex w-full max-w-md flex-col gap-6">
        <div className="flex flex-col items-center gap-2">
          <AnimatedLogo followMouse={false} />
          <h1 className="text-xl font-semibold tracking-wide">
            Choose your provider
          </h1>
          <p className="text-foreground-alt text-sm">
            How would you like to store your data?
          </p>
        </div>

        <div className="border-foreground/20 bg-background-get-started relative overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
          <div className="space-y-3 p-6">
            <RadioOption
              selected={false}
              onSelect={handleCloud}
              icon={<LuCloud sx={stylexStyles.rule1} />}
              label="Spacewave Cloud"
              tag="$8/mo"
              description="Cloud backup, multi-device sync, encrypted storage."
            />

            <button
              type="button"
              onClick={handleLocal}
              sx={[[stylexStyles2.rule1, groupMarker]]}
            >
              <LuHardDrive sx={stylexStyles.rule2} />
              <span className="text-foreground text-sm">
                Continue with local storage
              </span>
              <span className="text-foreground-alt text-xs">(Free)</span>
              <LuArrowRight sx={stylexStyles.rule2} />
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

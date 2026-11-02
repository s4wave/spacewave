import { useCallback, useMemo } from 'react'
import { LuArrowRight, LuX } from '@s4wave/web/images/icons.js'

import {
  useNavigate,
  useParentPaths,
  usePath,
} from '@s4wave/web/router/router.js'
import { useSessionStorageStats } from '@s4wave/app/session/SessionStorageStatsContext.js'
import { useLocalSessionOnboardingContext } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// SetupBanner renders a persistent inline banner while local session onboarding
// is incomplete. It stays hidden when the onboarding state cannot be read, as on
// a bound web listener, where setup is not available.
export function SetupBanner() {
  const navigate = useNavigate()
  const parentPaths = useParentPaths()
  const path = usePath()
  const basePath = parentPaths[parentPaths.length - 1] ?? path
  const onboarding = useLocalSessionOnboardingContext()
  const storage = useSessionStorageStats()

  const onSetupPage = path.startsWith(`${basePath}/setup`)

  const visible = useMemo(() => {
    if (!onboarding.available) return false
    if (!onboarding.metadataLoaded) return false
    if (storage.loading) return false
    if (!storage.setupBannerEligible) return false
    if (onSetupPage) return false
    if (onboarding.isComplete) return false
    return !onboarding.onboarding.dismissed
  }, [onboarding, onSetupPage, storage])

  const handleSetupClick = useCallback(() => {
    if (!onboarding.providerChoiceComplete) {
      navigate({ path: `${basePath}/plan` })
      return
    }
    if (!onboarding.onboarding.backupComplete) {
      navigate({ path: `${basePath}/setup` })
      return
    }
    if (!onboarding.onboarding.lockComplete) {
      navigate({ path: `${basePath}/setup` })
    }
  }, [
    navigate,
    onboarding.providerChoiceComplete,
    onboarding.onboarding.backupComplete,
    onboarding.onboarding.lockComplete,
    basePath,
  ])

  const handleDismiss = useCallback(() => {
    onboarding.dismiss()
  }, [onboarding])

  if (!visible) return null

  return (
    <div className="border-brand/20 bg-brand/5 relative flex items-center border-b">
      <button
        type="button"
        onClick={handleSetupClick}
        sx={[stylexStyles2.rule1, groupMarker2]}
      >
        <p className="text-foreground/80 group-hover:text-foreground text-xs font-medium transition-colors select-none">
          Finish setting up your account
        </p>
        <LuArrowRight sx={stylexStyles.rule1} />
      </button>
      <button
        type="button"
        onClick={handleDismiss}
        className="text-foreground-alt hover:text-foreground shrink-0 px-2 py-1.5 transition-colors"
        aria-label="Dismiss setup banner"
      >
        <LuX sx={stylexStyles.rule2} />
      </button>
    </div>
  )
}

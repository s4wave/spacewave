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

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    display: 'flex',
    position: 'relative',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 80%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
  },
  rule3: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    flexShrink: 0,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    cursor: 'pointer',
    display: 'flex',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minWidth: 0,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-foreground'],
      },
    },
    flexShrink: 0,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
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
    <div sx={stylexStyles3.rule1}>
      <button
        type="button"
        onClick={handleSetupClick}
        sx={[stylexStyles2.rule1, groupMarker2]}
      >
        <p sx={stylexStyles3.rule2}>Finish setting up your account</p>
        <LuArrowRight sx={stylexStyles.rule1} />
      </button>
      <button
        type="button"
        onClick={handleDismiss}
        sx={stylexStyles3.rule3}
        aria-label="Dismiss setup banner"
      >
        <LuX sx={stylexStyles.rule2} />
      </button>
    </div>
  )
}

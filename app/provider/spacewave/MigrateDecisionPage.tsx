import { useCallback, useState } from 'react'
import {
  LuArrowLeft,
  LuArrowRight,
  LuCloud,
  LuHardDrive,
  LuUnlink,
} from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import {
  CLOUD_FAQ,
  FaqAccordion,
  PageFooter,
  PageWrapper,
} from './CloudConfirmationPage.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../../web/style/relations.stylex.js'

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'flex-start',
    display: 'flex',
  },
  rule6: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule7: { flex: 1 },
  rule8: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule9: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: themeLayout['--spacing'],
  },
  rule10: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule11: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    '--tw-shadow': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover':
          '0 4px 6px -1px var(--tw-shadow-color, #0000001a), 0 2px 4px -2px var(--tw-shadow-color, #0000001a)',
      },
    },
    '--tw-shadow-color': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent) var(--tw-shadow-alpha), transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    boxShadow: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover':
          'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      },
    },
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
    textAlign: 'left',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    '--tw-duration': '.3s',
    '--tw-shadow': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover':
          '0 4px 6px -1px var(--tw-shadow-color, #0000001a), 0 2px 4px -2px var(--tw-shadow-color, #0000001a)',
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    boxShadow: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover':
          'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      },
    },
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
    textAlign: 'left',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-brand'],
      },
    },
    flexShrink: 0,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule5: {
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
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

// MigrateDecisionPage renders the migration decision UI at /plan/migrate.
// Shown when the user has an active subscription and a linked local session
// with content. Offers two choices: migrate data to cloud, or keep sessions
// separate (unlink the local session).
export function MigrateDecisionPage() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const navigate = useNavigate()
  const [unlinking, setUnlinking] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  const handleMigrate = useCallback(() => {
    navigate({ path: '../../settings/migration' })
  }, [navigate])

  const handleKeepSeparate = useCallback(async () => {
    if (!session) return
    setUnlinking(true)
    setError(null)
    try {
      await session.spacewave.unlinkLocalSession()
      navigate({ path: '../../' })
    } catch (err) {
      const msg =
        err instanceof Error ? err.message : 'Failed to unlink session'
      setError(msg)
      setUnlinking(false)
    }
  }, [session, navigate])

  return (
    <PageWrapper
      backButton={
        <button type="button" onClick={handleBack} sx={stylexStyles3.rule1}>
          <LuArrowLeft sx={stylexStyles.rule1} />
          Back
        </button>
      }
    >
      {/* Header */}
      <div sx={stylexStyles3.rule2}>
        <AnimatedLogo followMouse={false} />
        <h1 sx={stylexStyles3.rule3}>You have local data</h1>
        <p sx={stylexStyles3.rule4}>
          Your subscription is active. Choose what to do with your existing
          local session data.
        </p>
      </div>

      {/* Migrate card */}
      <button
        type="button"
        onClick={handleMigrate}
        disabled={unlinking}
        sx={[[stylexStyles2.rule1, groupMarker2]]}
      >
        <div sx={stylexStyles3.rule5}>
          <div sx={stylexStyles3.rule6}>
            <LuCloud sx={stylexStyles.rule2} />
          </div>
          <div sx={stylexStyles3.rule7}>
            <h2 sx={stylexStyles3.rule8}>Migrate my data to Cloud</h2>
            <p sx={stylexStyles3.rule9}>
              Move your local files and settings to Spacewave Cloud. Your data
              will be synced across all your devices.
            </p>
          </div>
          <LuArrowRight sx={stylexStyles.rule3} />
        </div>
      </button>

      {/* Keep separate card */}
      <button
        type="button"
        onClick={() => void handleKeepSeparate()}
        disabled={unlinking || !session}
        sx={[[stylexStyles2.rule2, groupMarker2]]}
      >
        <div sx={stylexStyles3.rule5}>
          <div sx={stylexStyles3.rule10}>
            {unlinking ? (
              <Spinner size="md" variant="muted" />
            ) : (
              <LuUnlink sx={stylexStyles.rule4} />
            )}
          </div>
          <div sx={stylexStyles3.rule7}>
            <h2 sx={stylexStyles3.rule8}>Keep sessions separate</h2>
            <p sx={stylexStyles3.rule9}>
              Unlink the local session from your cloud account. Both sessions
              remain accessible independently.
            </p>
          </div>
          <LuHardDrive sx={stylexStyles.rule5} />
        </div>
      </button>

      {error && <p sx={stylexStyles3.rule11}>{error}</p>}

      <FaqAccordion items={CLOUD_FAQ} />

      <PageFooter />
    </PageWrapper>
  )
}

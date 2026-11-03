import { useCallback, useState } from 'react'
import { LuMoon, LuSparkles, LuUser } from '@s4wave/web/images/icons.js'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'
import { findPersonalCanceledBillingAccount } from '../provider/spacewave/resubscribe-target.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: { padding: `calc(${themeLayout['--spacing']} * 6)` },
})

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-outline-style': 'none',
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    outlineStyle: 'none',
    position: 'relative',
    overflowY: 'auto',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-sm)',
    width: '100%',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 15%, transparent)`,
    color: themeColors['--color-warning'],
    fontSize: '9px',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
  rule8: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: themeColors['--color-background-get-started'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    position: 'relative',
  },
  rule9: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 12)`,
    width: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule10: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    textAlign: 'center',
  },
  rule11: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule12: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    textAlign: 'center',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    display: 'flex',
    justifyContent: 'center',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
  },
  rule2: {
    '--tw-duration': '.3s',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      },
    },
    display: 'flex',
    justifyContent: 'center',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-warning'],
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

export interface DormantOverlayProps {
  metadata: SessionMetadata
}

// DormantOverlay renders a full-page gate for cloud sessions whose account has
// entered the DORMANT state (subscription inactive or RBAC denied). Offers
// resubscribe as the primary action and returning to the session list as the
// secondary action.
export function DormantOverlay({ metadata }: DormantOverlayProps) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const { accountId } = useSessionInfo(session)
  const navigate = useNavigate()
  const navigateSession = useSessionNavigate()
  const [error, setError] = useState<string | null>(null)
  const [resolving, setResolving] = useState(false)

  const handleBack = useCallback(() => {
    navigate({ path: '/sessions' })
  }, [navigate])

  const handleReactivate = useCallback(async () => {
    if (!session) return

    setResolving(true)
    setError(null)
    try {
      const resp = await session.spacewave.listManagedBillingAccounts()
      const target = findPersonalCanceledBillingAccount(
        resp.accounts ?? [],
        accountId,
      )
      if (target?.id) {
        navigateSession({ path: `billing/${target.id}?reactivate=1` })
        return
      }
      navigateSession({ path: 'plan/no-active' })
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setResolving(false)
    }
  }, [accountId, navigateSession, session])

  const handleOpenLocal = useCallback(() => {
    navigate({ path: '/sessions' })
  }, [navigate])

  const entityId = metadata.displayName || metadata.cloudEntityId || ''
  const providerLabel = metadata.providerDisplayName || 'Cloud'

  return (
    <div sx={stylexStyles3.rule1}>
      <BackButton floating onClick={handleBack}>
        Sessions
      </BackButton>
      <div sx={stylexStyles3.rule2}>
        <div sx={stylexStyles3.rule3}>
          <AnimatedLogo followMouse={false} />
          <h1 sx={stylexStyles3.rule4}>Subscription inactive</h1>
          {entityId && (
            <div sx={stylexStyles3.rule5}>
              <span sx={stylexStyles3.rule6}>{entityId}</span>
              <span sx={stylexStyles3.rule7}>{providerLabel}</span>
            </div>
          )}
        </div>

        <div sx={stylexStyles3.rule8}>
          <div sx={stylexStyles4.rule1}>
            <div sx={[nativeChildSpacing.s4, stylexStyles3.rule3]}>
              <div sx={stylexStyles3.rule9}>
                <LuMoon sx={stylexStyles.rule1} />
              </div>
            </div>

            <p sx={[nativeChildSpacing.s4, stylexStyles3.rule10]}>
              This cloud session is paused because the subscription is not
              active. Reactivate the subscription to continue, or open a local
              session in the meantime.
            </p>

            <button
              type="button"
              onClick={() => {
                void handleReactivate()
              }}
              disabled={resolving}
              sx={[nativeChildSpacing.s4, [[stylexStyles2.rule1, groupMarker]]]}
            >
              <LuSparkles sx={stylexStyles.rule2} />
              <span sx={stylexStyles3.rule11}>
                {resolving ? 'Opening billing…' : 'Reactivate subscription'}
              </span>
            </button>

            {error && (
              <p sx={[nativeChildSpacing.s4, stylexStyles3.rule12]}>{error}</p>
            )}

            <button
              type="button"
              onClick={handleOpenLocal}
              sx={[nativeChildSpacing.s4, [[stylexStyles2.rule2, groupMarker]]]}
            >
              <LuUser sx={stylexStyles.rule2} />
              <span sx={stylexStyles3.rule11}>Open a local session</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
  },
})

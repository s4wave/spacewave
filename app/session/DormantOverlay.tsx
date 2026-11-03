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

const stylexStyles3 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
  rule2: {
    position: 'relative',
    zIndex: 10,
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-sm)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule3: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-foreground'],
  },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule7: {
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 15%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '9px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: themeColors['--color-warning'],
    textTransform: 'uppercase',
  },
  rule8: {
    position: 'relative',
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    backgroundColor: themeColors['--color-background-get-started'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule9: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 12)`,
    height: `calc(${themeLayout['--spacing']} * 12)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
  },
  rule10: {
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule12: {
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-destructive'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
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
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
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
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-warning'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
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
          <div
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: space-y-4 */ 'space-y-4 p-6'
            }
          >
            <div sx={stylexStyles3.rule3}>
              <div sx={stylexStyles3.rule9}>
                <LuMoon sx={stylexStyles.rule1} />
              </div>
            </div>

            <p sx={stylexStyles3.rule10}>
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
              sx={[[stylexStyles2.rule1, groupMarker]]}
            >
              <LuSparkles sx={stylexStyles.rule2} />
              <span sx={stylexStyles3.rule11}>
                {resolving ? 'Opening billing…' : 'Reactivate subscription'}
              </span>
            </button>

            {error && <p sx={stylexStyles3.rule12}>{error}</p>}

            <button
              type="button"
              onClick={handleOpenLocal}
              sx={[[stylexStyles2.rule2, groupMarker]]}
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

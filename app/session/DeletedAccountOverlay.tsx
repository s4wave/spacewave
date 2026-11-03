import { useCallback } from 'react'
import { LuCircleAlert } from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'

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
  rule5: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 12)`,
    width: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
  },
  rule7: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    textAlign: 'center',
  },
  rule9: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
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
    color: themeColors['--color-destructive'],
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
})

export interface DeletedAccountOverlayProps {
  metadata: SessionMetadata
  onRemove: () => Promise<void>
}

// DeletedAccountOverlay renders a full-page error screen for sessions whose cloud account was deleted.
export function DeletedAccountOverlay({
  metadata,
  onRemove,
}: DeletedAccountOverlayProps) {
  const navigate = useNavigate()

  const handleBack = useCallback(() => {
    navigate({ path: '/sessions' })
  }, [navigate])

  const handleRemove = useCallback(async () => {
    await onRemove()
    navigate({ path: '/sessions' })
  }, [onRemove, navigate])

  const handleRemoveClick = useCallback(() => {
    void handleRemove()
  }, [handleRemove])

  return (
    <div sx={stylexStyles3.rule1}>
      <BackButton floating onClick={handleBack}>
        Sessions
      </BackButton>
      <div sx={stylexStyles3.rule2}>
        <div sx={stylexStyles3.rule3}>
          <AnimatedLogo followMouse={false} />
        </div>

        <div sx={stylexStyles3.rule4}>
          <div sx={stylexStyles4.rule1}>
            <div sx={[nativeChildSpacing.s4, stylexStyles3.rule3]}>
              <div sx={stylexStyles3.rule5}>
                <LuCircleAlert sx={stylexStyles.rule1} />
              </div>
              <h2 sx={stylexStyles3.rule6}>
                {metadata.displayName || 'Deleted Session'}
              </h2>
              {metadata.providerDisplayName && (
                <p sx={stylexStyles3.rule7}>{metadata.providerDisplayName}</p>
              )}
            </div>

            <p sx={[nativeChildSpacing.s4, stylexStyles3.rule8]}>
              This account no longer exists on the cloud. The session cannot
              connect. You can remove it from your session list.
            </p>

            <button
              type="button"
              onClick={handleRemoveClick}
              sx={[nativeChildSpacing.s4, [[stylexStyles2.rule1, groupMarker]]]}
            >
              <span sx={stylexStyles3.rule9}>Remove Session</span>
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

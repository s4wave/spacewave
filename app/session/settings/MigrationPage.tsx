import { useCallback } from 'react'
import { LuArrowLeft, LuCpu } from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  motion as themeMotion,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { padding: `calc(${themeLayout['--spacing']} * 6)` },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
  },
  rule2: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-lg)',
  },
  rule3: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule4: { marginBottom: `calc(${themeLayout['--spacing']} * 6)` },
  rule5: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-foreground'],
  },
  rule6: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule7: {
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
  rule8: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule9: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule10: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule11: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule12: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
  },
  rule13: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule14: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-foreground-alt'],
  },
})

// MigrationPage renders a placeholder settings page for provider migration.
// Shows the current provider type and a coming-soon message.
export function MigrationPage() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const navigate = useNavigate()

  const { providerId } = useSessionInfo(session)
  const isLocal = providerId === 'local'

  const handleBack = useCallback(() => {
    navigate({ path: '../../' })
  }, [navigate])

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <button type="button" onClick={handleBack} sx={stylexStyles2.rule3}>
          <LuArrowLeft sx={stylexStyles.rule1} />
          Back to dashboard
        </button>

        <div sx={stylexStyles2.rule4}>
          <h1 sx={stylexStyles2.rule5}>Provider Migration</h1>
          <p sx={stylexStyles2.rule6}>Manage your data storage provider.</p>
        </div>

        <div sx={stylexStyles2.rule7}>
          <div sx={stylexStyles3.rule1}>
            <div sx={[nativeChildSpacing.s4, stylexStyles2.rule8]}>
              <div sx={stylexStyles2.rule9}>
                <LuCpu sx={stylexStyles.rule2} />
              </div>
              <div>
                <p sx={stylexStyles2.rule10}>Current provider</p>
                <p sx={stylexStyles2.rule11}>
                  {isLocal ? 'Local (browser storage)' : 'Spacewave Cloud'}
                </p>
              </div>
            </div>

            <div sx={[nativeChildSpacing.s4, [stylexStyles2.rule12]]}>
              <p sx={stylexStyles2.rule13}>
                Provider migration is coming soon.
              </p>
              <p sx={stylexStyles2.rule14}>
                You will be able to switch between Local and Spacewave Cloud
                providers here.
              </p>
            </div>
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
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

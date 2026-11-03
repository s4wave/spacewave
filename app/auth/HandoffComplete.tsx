import { useCallback } from 'react'
import {
  LuArrowRight,
  LuCheck,
  LuLaptop,
  LuUser,
} from '@s4wave/web/images/icons.js'

import type { HandoffRequest } from '@s4wave/core/session/handoff/handoff.pb.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { clientTypeLabel } from './handoff-state.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  utilityState as themeUtilityState,
  radii as themeRadii,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule2: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    whiteSpace: 'nowrap',
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule4: {
    position: 'relative',
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    backgroundColor: themeColors['--color-background-landing'],
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule5: {
    position: 'relative',
    zIndex: 10,
    display: 'flex',
    maxWidth: 'var(--container-sm)',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
  },
  rule6: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 16)`,
    height: `calc(${themeLayout['--spacing']} * 16)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule7: {
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
  },
  rule8: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 90%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    color: themeColors['--color-brand'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

// detailChipClassName styles one handoff detail chip.

// HandoffDetails renders the account and device of a handoff as chips that
// wrap whole, so a long username or host name never splits across lines.
export function HandoffDetails({
  username,
  deviceName,
}: {
  username?: string
  deviceName: string
}) {
  if (!username && !deviceName) return null
  return (
    <div sx={stylexStyles2.rule1}>
      {username && (
        <span sx={stylexStyles2.rule2}>
          <LuUser sx={stylexStyles.rule1} />
          <span sx={stylexStyles2.rule3}>{username}</span>
        </span>
      )}
      {deviceName && (
        <span sx={stylexStyles2.rule2}>
          <LuLaptop sx={stylexStyles.rule1} />
          {deviceName}
        </span>
      )}
    </div>
  )
}

// HandoffComplete confirms that a CLI or desktop handoff finished and offers
// the browser Session the person signed in to along the way.
export function HandoffComplete({
  request,
  sessionIndex,
}: {
  request: HandoffRequest
  sessionIndex: number
}) {
  const navigate = useNavigate()
  const label = clientTypeLabel(request.clientType ?? '')
  const handleOpen = useCallback(() => {
    navigate({ path: `/u/${sessionIndex}` })
  }, [navigate, sessionIndex])

  return (
    <div sx={stylexStyles2.rule4}>
      <div sx={stylexStyles2.rule5}>
        <div sx={stylexStyles2.rule6}>
          <LuCheck sx={stylexStyles.rule2} />
        </div>
        <h1 sx={stylexStyles2.rule7}>Sign-in complete</h1>
        <HandoffDetails deviceName={request.deviceName ?? ''} />
        <p sx={stylexStyles2.rule8}>
          Spacewave {label} is signed in. Return to it, or keep going here in
          the browser.
        </p>
        <button type="button" onClick={handleOpen} sx={stylexStyles2.rule9}>
          Open Spacewave
          <LuArrowRight sx={stylexStyles.rule3} />
        </button>
      </div>
    </div>
  )
}

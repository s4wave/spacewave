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
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexWrap: 'wrap',
    fontSize: themeFonts['--text-sm'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
    display: 'inline-flex',
    whiteSpace: 'nowrap',
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule4: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    position: 'relative',
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    textAlign: 'center',
    zIndex: 10,
    maxWidth: 'var(--container-sm)',
  },
  rule6: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 16)`,
    width: `calc(${themeLayout['--spacing']} * 16)`,
  },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
  },
  rule8: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule9: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: themeRadii['--radius-md'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 90%, transparent)`,
      },
    },
    color: themeColors['--color-brand-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
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

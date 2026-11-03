import { useCallback } from 'react'
import { LuBox } from '@s4wave/web/images/icons.js'
import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import type { OrgSpaceInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    padding: themeLayout['--spacing'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    borderRadius: themeRadii['--radius-md'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    display: 'flex',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule3: { flex: 1, minWidth: 0 },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule5: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// OrgSpaceList renders the list of spaces owned by an organization.
export function OrgSpaceList(props: { orgId: string; spaces: OrgSpaceInfo[] }) {
  return (
    <div>
      {props.spaces.length === 0 && (
        <div sx={[nativeChildSpacing.s1, stylexStyles2.rule1]}>
          <LuBox sx={stylexStyles.rule1} />
          <span>No spaces yet</span>
        </div>
      )}
      {props.spaces.map((space) => (
        <OrgSpaceRow
          sx={nativeChildSpacing.s1}
          key={space.id}
          orgId={props.orgId}
          space={space}
        />
      ))}
    </div>
  )
}

function OrgSpaceRow(
  props: { orgId: string; space: OrgSpaceInfo } & {
    sx?: stylex.StyleXStyles
    className?: never
  },
) {
  const { orgId, space } = props
  const navigateSession = useSessionNavigate()

  const handleSpaceSelect = useCallback(() => {
    navigateSession({ path: `org/${orgId}/so/${space.id}` })
  }, [navigateSession, orgId, space.id])

  return (
    <button
      type="button"
      onClick={handleSpaceSelect}
      sx={[stylexStyles2.rule2, props.sx]}
    >
      <LuBox sx={stylexStyles.rule2} />
      <div sx={stylexStyles2.rule3}>
        <div sx={stylexStyles2.rule4}>{space.displayName || space.id}</div>
      </div>
      {space.objectType && (
        <span sx={stylexStyles2.rule5}>{space.objectType}</span>
      )}
    </button>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
  },
})

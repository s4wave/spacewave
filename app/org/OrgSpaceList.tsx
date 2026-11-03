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
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    padding: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule2: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule3: { minWidth: 0, flex: 1 },
  rule4: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
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
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

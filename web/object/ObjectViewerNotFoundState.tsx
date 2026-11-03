import { LuCircleAlert } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: themeColors['--color-background-primary'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    display: 'flex',
    maxWidth: 'var(--container-sm)',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule5: { minWidth: 0 },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule7: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    overflowWrap: 'break-word',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

export interface ObjectViewerNotFoundStateProps {
  objectKey?: string
}

// ObjectViewerNotFoundState renders the shared missing-object viewer surface.
export function ObjectViewerNotFoundState({
  sx,
  objectKey,
}: ObjectViewerNotFoundStateProps & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  return (
    <div sx={[stylexStyles.rule1, sx]}>
      <div sx={stylexStyles.rule2}>
        <div sx={stylexStyles.rule3}>
          <LuCircleAlert sx={stylexStyles.rule4} />
        </div>
        <div sx={stylexStyles.rule5}>
          <p sx={stylexStyles.rule6}>Object not found</p>
          <p sx={stylexStyles.rule7}>
            {objectKey || 'The selected object'} is no longer available.
          </p>
        </div>
      </div>
    </div>
  )
}

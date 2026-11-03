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
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
    width: '100%',
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    display: 'flex',
    maxWidth: 'var(--container-sm)',
  },
  rule3: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule4: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: { minWidth: 0 },
  rule6: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule7: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    overflowWrap: 'break-word',
    marginTop: themeLayout['--spacing'],
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

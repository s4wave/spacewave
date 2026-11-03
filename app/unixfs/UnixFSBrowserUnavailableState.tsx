import type { ComponentProps } from 'react'
import { LuCircleAlert, LuRotateCw } from '@s4wave/web/images/icons.js'

import { UnixFSBrowserShell } from './UnixFSBrowserShell.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule2: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 80%, transparent)`,
  },
  rule3: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
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
  rule4: {
    display: 'flex',
    minHeight: 0,
    flex: 1,
    overflow: 'hidden',
    backgroundColor: themeColors['--color-file-back'],
  },
  rule5: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'hidden',
  },
  rule6: {
    display: 'flex',
    minHeight: 0,
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    overflow: 'hidden',
    backgroundColor: themeColors['--color-file-back'],
  },
  rule7: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-destructive'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

type UnixFSBrowserShellProps = Omit<
  ComponentProps<typeof UnixFSBrowserShell>,
  'children'
>

type UnixFSBrowserUnavailableStateProps =
  | {
      kind: 'error'
      shellProps: UnixFSBrowserShellProps
      error: Error
      onRetry: () => void
    }
  | {
      kind: 'not-found'
      unixfsId: string
    }

export function UnixFSBrowserUnavailableState(
  props: UnixFSBrowserUnavailableStateProps,
) {
  if (props.kind === 'error') {
    return (
      <UnixFSBrowserShell {...props.shellProps}>
        <div sx={stylexStyles2.rule1}>
          <LuCircleAlert sx={stylexStyles.rule1} />
          <p sx={stylexStyles2.rule2}>
            Error loading files: {props.error.message ?? 'Unknown error'}
          </p>
          <button
            type="button"
            onClick={props.onRetry}
            sx={stylexStyles2.rule3}
          >
            <LuRotateCw sx={stylexStyles.rule2} />
            Retry
          </button>
        </div>
        <div sx={stylexStyles2.rule4} />
      </UnixFSBrowserShell>
    )
  }

  return (
    <div data-testid="unixfs-browser" sx={stylexStyles2.rule5}>
      <div sx={stylexStyles2.rule6}>
        <div sx={stylexStyles2.rule7}>UnixFS object not found</div>
        <div sx={stylexStyles2.rule8}>Object: {props.unixfsId || 'none'}</div>
      </div>
    </div>
  )
}

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
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule2: {
    flex: 1,
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 80%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'inline-flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule4: {
    flex: 1,
    overflow: 'hidden',
    backgroundColor: themeColors['--color-file-back'],
    display: 'flex',
    minHeight: 0,
  },
  rule5: {
    overflow: 'hidden',
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule6: {
    flex: 1,
    overflow: 'hidden',
    alignItems: 'center',
    backgroundColor: themeColors['--color-file-back'],
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    minHeight: 0,
  },
  rule7: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-destructive'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
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

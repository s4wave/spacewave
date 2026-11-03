import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import type React from 'react'

import spacewaveIcon from '@s4wave/web/images/spacewave-icon.png'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-outline-style': 'none',
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    containerType: 'inline-size',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: {
      default: 'center',
      '@container (max-height: 580px)': 'flex-start',
    },
    outlineStyle: 'none',
    position: 'relative',
    overflowY: 'auto',
    width: '100%',
  },
  rule2: {
    position: 'absolute',
    zIndex: 20,
    left: `calc(${themeLayout['--spacing']} * 4)`,
    top: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    position: 'absolute',
    zIndex: 20,
    right: `calc(${themeLayout['--spacing']} * 4)`,
    top: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    position: 'absolute',
    zIndex: 20,
    bottom: `calc(${themeLayout['--spacing']} * 4)`,
    right: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: {
      default: null,
      '@container (max-height: 580px)': 'flex-start',
      '@container (max-height: 740px)': 'center',
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: {
      default: 'var(--container-5xl)',
      '@container (max-height: 740px)': 'none',
    },
    minHeight: { default: null, '@container (max-height: 740px)': '100%' },
    paddingBottom: {
      default: null,
      '@container (max-height: 580px)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingTop: {
      default: null,
      '@container (max-height: 580px)': `calc(${themeLayout['--spacing']} * 16)`,
    },
    width: { default: '100%', '@container (max-height: 740px)': '100%' },
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: {
      default: 'flex',
      '@container (max-height: 740px)': 'none',
      '@container (max-height: 740px) and (max-width: 950px)': 'none',
    },
    flexDirection: 'column',
    textAlign: 'center',
    maxWidth: 'var(--container-sm)',
    width: '100%',
  },
  rule9: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    textAlign: 'center',
    maxWidth: 'var(--container-sm)',
    width: '100%',
  },
  rule10: {
    marginInline: { default: null, '@container (max-height: 740px)': 'auto' },
    flexShrink: 0,
    maxWidth: {
      default: 'var(--container-sm)',
      '@container (max-height: 740px)': 'var(--container-md)',
    },
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    gap: {
      default: null,
      '@container (max-height: 740px)': `calc(${themeLayout['--spacing']} * 2)`,
    },
    alignItems: { default: null, '@container (max-height: 740px)': 'center' },
    display: {
      default: 'none',
      '@container (max-height: 740px)': 'flex',
      '@container (max-height: 740px) and (max-width: 950px)': 'none',
    },
    position: { default: null, '@container (max-height: 740px)': 'absolute' },
    bottom: {
      default: null,
      '@container (max-height: 740px)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    left: {
      default: null,
      '@container (max-height: 740px)': `calc(${themeLayout['--spacing']} * 4)`,
    },
  },
  rule2: {
    borderRadius: 'var(--radius-sm)',
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

export interface AuthScreenLayoutProps extends Omit<
  React.ComponentPropsWithRef<'div'>,
  'className'
> {
  className?: never
  sx?: ComponentStyles
  intro: React.ReactNode
  renderTopLeft?: (buttonSx: stylex.StyleXStyles) => React.ReactNode
  renderTopRight?: (buttonSx: stylex.StyleXStyles) => React.ReactNode
  renderBottomRight?: (buttonSx: stylex.StyleXStyles) => React.ReactNode
  alwaysShowIntro?: boolean
  introSx?: stylex.StyleXStyles
  contentSx?: stylex.StyleXStyles
  shellSx?: stylex.StyleXStyles
}

// AuthScreenLayout renders the shared auth page shell with container-aware
// intro and form placement.
export function AuthScreenLayout({
  intro,
  renderTopLeft,
  renderTopRight,
  renderBottomRight,
  alwaysShowIntro,
  introSx,
  contentSx,
  shellSx,
  children,
  sx,
  ...props
}: Omit<AuthScreenLayoutProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <div {...stylexProps([stylexStyles2.rule1, sx], props)}>
      {renderTopLeft && (
        <div sx={stylexStyles2.rule2}>
          {renderTopLeft(authCornerStyles.button)}
        </div>
      )}
      {renderTopRight && (
        <div sx={stylexStyles2.rule3}>
          {renderTopRight(authCornerStyles.button)}
        </div>
      )}
      {renderBottomRight && (
        <div sx={stylexStyles2.rule4}>
          {renderBottomRight(authCornerStyles.button)}
        </div>
      )}

      <div sx={[[stylexStyles2.rule5], shellSx]}>
        <div
          sx={[
            alwaysShowIntro ? stylexStyles2.rule9 : stylexStyles2.rule6,
            introSx,
          ]}
        >
          {intro}
        </div>
        {!alwaysShowIntro && (
          <div sx={stylexStyles.rule1}>
            <img
              src={spacewaveIcon}
              alt="Spacewave Icon"
              sx={stylexStyles.rule2}
            />
            <span sx={stylexStyles.rule3}>Spacewave</span>
          </div>
        )}
        <div sx={[[stylexStyles2.rule10], contentSx]}>{children}</div>
      </div>
    </div>
  )
}

// authCornerStyles applies coarse-pointer sizing at each actual corner button.
const authCornerStyles = stylex.create({
  button: {
    paddingInline: {
      default: null,
      '@media (pointer: coarse)': 'calc(var(--spacing) * 2)',
    },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': 'calc(var(--spacing) * 11)',
    },
  },
})

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
    containerType: 'inline-size',
    position: 'relative',
    display: 'flex',
    width: '100%',
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: {
      default: 'center',
      '@container (max-height: 580px)': 'flex-start',
    },
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
  rule2: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 4)`,
    left: `calc(${themeLayout['--spacing']} * 4)`,
    zIndex: 20,
  },
  rule3: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 4)`,
    right: `calc(${themeLayout['--spacing']} * 4)`,
    zIndex: 20,
  },
  rule4: {
    position: 'absolute',
    right: `calc(${themeLayout['--spacing']} * 4)`,
    bottom: `calc(${themeLayout['--spacing']} * 4)`,
    zIndex: 20,
  },
  rule5: {
    position: 'relative',
    zIndex: 10,
    display: 'flex',
    width: { default: '100%', '@container (max-height: 740px)': '100%' },
    maxWidth: {
      default: 'var(--container-5xl)',
      '@container (max-height: 740px)': 'none',
    },
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    minHeight: { default: null, '@container (max-height: 740px)': '100%' },
    justifyContent: {
      default: null,
      '@container (max-height: 740px)': 'center',
      '@container (max-height: 580px)': 'flex-start',
    },
    paddingTop: {
      default: null,
      '@container (max-height: 580px)': `calc(${themeLayout['--spacing']} * 16)`,
    },
    paddingBottom: {
      default: null,
      '@container (max-height: 580px)': `calc(${themeLayout['--spacing']} * 8)`,
    },
  },
  rule6: {
    display: {
      default: 'flex',
      '@container (max-height: 740px)': 'none',
      '@container (max-height: 740px) and (max-width: 950px)': 'none',
    },
    width: '100%',
    maxWidth: 'var(--container-sm)',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
  },
  rule7: {
    display: { default: 'flex', '@container (max-height: 740px)': 'none' },
    width: '100%',
    maxWidth: 'var(--container-sm)',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
  },
  rule8: {
    display: {
      default: 'flex',
      '@container (max-height: 740px) and (max-width: 950px)': 'none',
    },
    width: '100%',
    maxWidth: 'var(--container-sm)',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
  },
  rule9: {
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-sm)',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
  },
  rule10: {
    width: '100%',
    maxWidth: {
      default: 'var(--container-sm)',
      '@container (max-height: 740px)': 'var(--container-md)',
    },
    flexShrink: 0,
    marginInline: { default: null, '@container (max-height: 740px)': 'auto' },
  },
})

const stylexStyles = stylex.create({
  rule1: {
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
    alignItems: { default: null, '@container (max-height: 740px)': 'center' },
    gap: {
      default: null,
      '@container (max-height: 740px)': `calc(${themeLayout['--spacing']} * 2)`,
    },
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: 'var(--radius-sm)',
  },
  rule3: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-foreground'],
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
    minHeight: {
      default: null,
      '@media (pointer: coarse)': 'calc(var(--spacing) * 11)',
    },
    paddingInline: {
      default: null,
      '@media (pointer: coarse)': 'calc(var(--spacing) * 2)',
    },
  },
})

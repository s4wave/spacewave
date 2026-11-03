import { createPortal } from 'react-dom'

import { isDesktop, isMac } from '@aptre/bldr'
import {
  useIsStaticMode,
  useStaticHref,
} from '@s4wave/app/prerender/StaticContext.js'
import { useAppBuildInfo } from '@s4wave/app/build-info.js'
import { ExternalLink } from './ExternalLink.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: {
      default: null,
      '@media (max-width: 640px)': 'none',
      '@media (pointer: coarse)': 'none',
    },
    fontSize: '10px',
    lineHeight: 1.5,
    pointerEvents: 'none',
    transitionDuration: '.3s',
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
    zIndex: 50,
  },
  rule2: { position: 'fixed' },
  rule3: {
    left: `calc(${themeLayout['--spacing']} * 18.25)`,
    top: `calc(${themeLayout['--spacing']} * 1.75)`,
  },
  rule4: {
    left: `calc(${themeLayout['--spacing']} * 4)`,
    top: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: { opacity: 0.3 },
  rule6: { opacity: 0 },
  rule7: { position: 'fixed', right: `calc(${themeLayout['--spacing']} * 4)` },
  rule8: { top: `calc(${themeLayout['--spacing']} * 1.75)` },
  rule9: { top: `calc(${themeLayout['--spacing']} * 2)` },
  rule10: {
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: {
      default: null,
      '@media (max-width: 640px)': 'none',
      '@media (pointer: coarse)': 'none',
    },
    fontSize: '10px',
    lineHeight: 1.5,
    opacity: 0.3,
    pointerEvents: 'auto',
    position: 'fixed',
    transitionDuration: '.3s',
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
    zIndex: 50,
    bottom: `calc(${themeLayout['--spacing']} * 2)`,
    right: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule11: {
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: {
      default: null,
      '@media (max-width: 640px)': 'none',
      '@media (pointer: coarse)': 'none',
    },
    fontSize: '10px',
    lineHeight: 1.5,
    opacity: 0,
    pointerEvents: 'auto',
    position: 'fixed',
    transitionDuration: '.3s',
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
    zIndex: 50,
    bottom: `calc(${themeLayout['--spacing']} * 2)`,
    right: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: {
      default: null,
      '@media (max-width: 640px)': 'none',
      '@media (pointer: coarse)': 'none',
    },
    fontSize: '10px',
    lineHeight: 1.5,
    opacity: 0.3,
    pointerEvents: 'auto',
    position: 'fixed',
    transitionDuration: '.3s',
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
    zIndex: 50,
    bottom: `calc(${themeLayout['--spacing']} * 2)`,
    left: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: {
      default: null,
      '@media (max-width: 640px)': 'none',
      '@media (pointer: coarse)': 'none',
    },
    fontSize: '10px',
    lineHeight: 1.5,
    opacity: 0,
    pointerEvents: 'auto',
    position: 'fixed',
    transitionDuration: '.3s',
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
    zIndex: 50,
    bottom: `calc(${themeLayout['--spacing']} * 2)`,
    left: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

interface CornerTextProps {
  show: boolean
}

// CornerText renders corner text for the landing page. A touch screen hides it:
// the page footer carries the same links at touch size.
// Desktop: uses fixed positioning via portal (for window chrome integration)
// Browser: uses fixed positioning to stay in viewport corners
export function CornerText({ show }: CornerTextProps) {
  const isStatic = useIsStaticMode()
  const dmcaHref = useStaticHref('/dmca')
  const buildInfo = useAppBuildInfo()
  const showTabHint = isStatic || isDesktop

  // Desktop offsets account for window chrome: left hint clears the macOS
  // traffic lights (cluster ends ~66px in), right hint mirrors its top edge so
  // both corners share one baseline. left-[63px] overlapped the zoom button.

  const content = (
    <>
      {showTabHint && (
        <span
          sx={[
            stylexStyles2.rule1,
            stylexStyles2.rule2,
            isDesktop && isMac ? stylexStyles2.rule3 : stylexStyles2.rule4,
            show ? stylexStyles2.rule5 : stylexStyles2.rule6,
          ]}
        >
          press tab to navigate
        </span>
      )}

      <div
        sx={[
          stylexStyles2.rule1,
          stylexStyles2.rule7,
          isDesktop && isMac ? stylexStyles2.rule8 : stylexStyles2.rule9,
          show ? stylexStyles2.rule5 : stylexStyles2.rule6,
        ]}
      >
        {buildInfo.cornerLabel}
      </div>

      <ExternalLink
        href="https://aperture.us"
        sx={show ? stylexStyles.rule1 : stylexStyles.rule2}
      >
        {/* The cake is a lie! */}
        <strong>Aperture Robotics</strong>
      </ExternalLink>

      <a
        href={dmcaHref}
        sx={show ? stylexStyles2.rule10 : stylexStyles2.rule11}
      >
        DMCA and legal
      </a>
    </>
  )

  // Desktop: portal to body for window chrome integration
  if (isDesktop) {
    return createPortal(content, document.body)
  }
  return content
}

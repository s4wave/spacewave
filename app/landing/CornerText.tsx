import { createPortal } from 'react-dom'
import { cn } from '@s4wave/web/style/utils.js'
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

const stylexStyles = stylex.create({
  rule1: {
    pointerEvents: 'auto',
    position: 'fixed',
    bottom: `calc(${themeLayout['--spacing']} * 2)`,
    left: `calc(${themeLayout['--spacing']} * 4)`,
    zIndex: 50,
    fontSize: '10px',
    lineHeight: 1.5,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    opacity: 0.3,
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    display: {
      default: null,
      '@media (max-width: 640px)': 'none',
      '@media (pointer: coarse)': 'none',
    },
  },
  rule2: {
    pointerEvents: 'auto',
    position: 'fixed',
    bottom: `calc(${themeLayout['--spacing']} * 2)`,
    left: `calc(${themeLayout['--spacing']} * 4)`,
    zIndex: 50,
    fontSize: '10px',
    lineHeight: 1.5,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    opacity: 0,
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    display: {
      default: null,
      '@media (max-width: 640px)': 'none',
      '@media (pointer: coarse)': 'none',
    },
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
  const baseClasses =
    'text-foreground-alt pointer-events-none z-50 micro-ten transition-opacity duration-300 select-none narrow:hidden [@media(pointer:coarse)]:hidden'
  const opacityClass = show ? 'opacity-30' : 'opacity-0'

  // Desktop offsets account for window chrome: left hint clears the macOS
  // traffic lights (cluster ends ~66px in), right hint mirrors its top edge so
  // both corners share one baseline. left-[63px] overlapped the zoom button.
  const topLeftPos = isDesktop && isMac ? 'top-1.75 left-18.25' : 'top-2 left-4'
  const topRightPos = isDesktop && isMac ? 'top-1.75' : 'top-2'

  const content = (
    <>
      {showTabHint && (
        <span className={cn(baseClasses, 'fixed', topLeftPos, opacityClass)}>
          press tab to navigate
        </span>
      )}

      <div
        className={cn(baseClasses, 'fixed right-4', topRightPos, opacityClass)}
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
        className={cn(
          baseClasses,
          'hover:text-foreground pointer-events-auto fixed right-4 bottom-2',
          opacityClass,
        )}
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

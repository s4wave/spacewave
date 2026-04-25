import { createPortal } from 'react-dom'
import { cn } from '@s4wave/web/style/utils.js'
import { isDesktop, isMac } from '@aptre/bldr'
import {
  useIsStaticMode,
  useStaticHref,
} from '@s4wave/app/prerender/StaticContext.js'
import { useAppBuildInfo } from '@s4wave/app/build-info.js'

interface CornerTextProps {
  show: boolean
}

// CornerText renders corner text for the landing page.
// Desktop: uses fixed positioning via portal (for window chrome integration)
// Browser: uses fixed positioning to stay in viewport corners
export function CornerText({ show }: CornerTextProps) {
  const isStatic = useIsStaticMode()
  const dmcaHref = useStaticHref('/dmca')
  const buildInfo = useAppBuildInfo()
  const showTabHint = isStatic || isDesktop
  const baseClasses =
    'text-foreground-alt pointer-events-none z-50 text-[10px] transition-opacity duration-300 select-none'
  const opacityClass = show ? 'opacity-30' : 'opacity-0'

  // Desktop offsets account for window chrome (traffic lights on mac)
  const topLeftPos =
    isDesktop && isMac ? 'top-[7px] left-[68px]' : 'top-2 left-4'
  const topRightPos = isDesktop && isMac ? 'top-[7px]' : 'top-2'

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

      <a
        href="https://aperture.us"
        className={cn(
          baseClasses,
          'hover:text-foreground pointer-events-auto fixed bottom-2 left-4',
          opacityClass,
        )}
      >
        {/* The cake is a lie! */}
        <strong>Aperture Robotics</strong>
      </a>

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

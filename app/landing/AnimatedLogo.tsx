import { useMemo, useEffect, useState } from 'react'
import { useDocumentVisibility } from '@aptre/bldr-react'
import { useMouse } from '@uidotdev/usehooks'

import { useIsTabActive } from '@s4wave/app/ShellTabContext.js'
import { cn } from '@s4wave/web/style/utils.js'
import spacewaveIcon from '@s4wave/web/images/spacewave-icon.png'

import './AnimatedLogo.css'

// extStyle accepts a style object with extended CSS properties (such as
// dynamicRangeLimit) and returns it as React.CSSProperties. Centralizes the
// single type widening needed for non-standard CSS properties.
function extStyle(s: Record<string, unknown>): React.CSSProperties {
  return s
}

const AnimatedLogo = ({
  className,
  containerClassName,
  followMouse = true,
}: {
  className?: string
  containerClassName?: string
  followMouse?: boolean
}) => {
  const [mouse, mouseRef] = useMouse<HTMLDivElement>()
  const docVisible = useDocumentVisibility()
  const isTabActive = useIsTabActive()
  const [elementRect, setElementRect] = useState<DOMRect | null>(null)
  const [isOnScreen, setIsOnScreen] = useState(false)
  const [hasMouse, setHasMouse] = useState(false)
  const canRunAnimation = isOnScreen && docVisible === 'visible' && isTabActive
  const canAnimate = followMouse && canRunAnimation && hasMouse

  useEffect(() => {
    const el = mouseRef.current
    if (!el) return

    const observer = new IntersectionObserver(([entry]) => {
      setIsOnScreen(entry?.isIntersecting ?? false)
    })
    observer.observe(el)

    return () => {
      observer.disconnect()
    }
  }, [mouseRef])

  useEffect(() => {
    const query = window.matchMedia('(hover: hover) and (pointer: fine)')
    const update = () => setHasMouse(query.matches)
    update()
    query.addEventListener('change', update)

    return () => {
      query.removeEventListener('change', update)
    }
  }, [])

  useEffect(() => {
    if (!canAnimate || !mouseRef.current) return

    const updateRect = () => {
      if (mouseRef.current) {
        setElementRect(mouseRef.current.getBoundingClientRect())
      }
    }

    updateRect()

    const resizeObserver = new ResizeObserver(updateRect)
    resizeObserver.observe(mouseRef.current)

    return () => {
      resizeObserver.disconnect()
    }
  }, [canAnimate, mouseRef])

  const mousePosition = useMemo(() => {
    if (!canAnimate || !elementRect) return { x: 0, y: 0, distance: 0 }

    const elementCenterX = elementRect.left + elementRect.width / 2
    const elementCenterY = elementRect.top + elementRect.height / 2

    const x = ((mouse.x ?? 0) / window.innerWidth) * 2 - 1
    const y = ((mouse.y ?? 0) / window.innerHeight) * 2 - 1

    const dx = (mouse.x ?? 0) - elementCenterX
    const dy = (mouse.y ?? 0) - elementCenterY
    const distance = Math.min(Math.hypot(dx, dy) / 1000, 1)

    return { x, y, distance }
  }, [mouse.x, mouse.y, canAnimate, elementRect])

  const transform = useMemo(
    () => ({
      rotateX: canAnimate ? mousePosition.y * -9.262 : 0, // 8.42 * 1.1
      rotateY: canAnimate ? mousePosition.x * 8.42 : 0,
      scale: canAnimate ? 1 + mousePosition.distance * 0.002 : 1,
    }),
    [mousePosition, canAnimate],
  )

  return (
    <div
      ref={mouseRef}
      className={cn('group relative perspective-[1000px]', containerClassName)}
    >
      <div
        className="relative size-20 @lg:h-28 @lg:w-28"
        style={{
          transform: `rotateX(${transform.rotateX}deg) rotateY(${transform.rotateY}deg) scale(${transform.scale})`,
          transition: 'transform 0.8s ease-out',
        }}
      >
        {/* Background Gradient Layer - Blur for Depth */}
        <div
          className={cn(
            'absolute -inset-[2px] z-[1] rounded-3xl opacity-50 blur-md transition duration-800 will-change-transform group-hover:scale-105 group-hover:opacity-55',
            'bg-[radial-gradient(circle_farthest-corner_at_100%_0,var(--color-brand),transparent),radial-gradient(circle_farthest-corner_at_0_100%,var(--color-logo-blue),transparent),radial-gradient(circle_farthest-corner_at_0_0,var(--color-brand),transparent),radial-gradient(circle_at_50%_50%,var(--color-logo-base)_10%,var(--color-logo-dark)_80%)]',
            canRunAnimation && 'animate-[pulse_10s_ease-in-out_infinite]',
          )}
          style={extStyle({
            animationName: canRunAnimation ? 'logoBlur' : 'none',
            animationDuration: '10s',
            animationIterationCount: 'infinite',
            animationTimingFunction: 'ease-in-out',
            animationPlayState: canRunAnimation ? 'running' : 'paused',
            dynamicRangeLimit: 'no-limit',
          })}
        />

        {/* Background Gradient Layer - Sharp for Clean Border */}
        <div
          className={cn(
            'absolute -inset-[1px] z-[2] rounded-3xl will-change-transform',
            'bg-[radial-gradient(circle_farthest-corner_at_100%_0,var(--color-brand),transparent),radial-gradient(circle_farthest-corner_at_0_100%,var(--color-logo-blue),transparent),radial-gradient(circle_farthest-corner_at_0_0,var(--color-brand),transparent),radial-gradient(circle_at_50%_50%,var(--color-logo-base)_10%,var(--color-logo-dark)_80%)]',
          )}
        />

        {/* Logo Image - Ensures it stays on top */}
        <div
          className={cn(
            'relative z-10 h-full w-full overflow-hidden rounded-3xl',
            className,
          )}
        >
          <img
            src={spacewaveIcon}
            alt="Spacewave Icon"
            className="h-full w-full max-w-none"
          />
        </div>
      </div>
    </div>
  )
}

export default AnimatedLogo

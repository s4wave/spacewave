import { useMemo, useEffect, useState, type RefObject } from 'react'
import { useDocumentVisibility } from '@aptre/bldr-react'
import { useMouse } from '@uidotdev/usehooks'

import { useIsTabActive } from '@s4wave/app/ShellTabContext.js'
import { cn } from '@s4wave/web/style/utils.js'
import spacewaveIcon from '@s4wave/web/images/spacewave-icon.png'

import './AnimatedLogo.css'

function rectEquals(a: DOMRect | null, b: DOMRect): boolean {
  return (
    a !== null &&
    a.left === b.left &&
    a.top === b.top &&
    a.width === b.width &&
    a.height === b.height
  )
}

/** useIsOnScreen reports whether the element is intersecting the viewport. */
function useIsOnScreen(
  ref: RefObject<HTMLElement | null>,
  enabled: boolean,
): boolean {
  const [isOnScreen, setIsOnScreen] = useState(false)

  useEffect(() => {
    if (!enabled) return

    const el = ref.current
    if (!el) return

    const observer = new IntersectionObserver(([entry]) => {
      const next = entry?.isIntersecting ?? false
      setIsOnScreen((prev) => (prev === next ? prev : next))
    })
    observer.observe(el)

    return () => {
      observer.disconnect()
    }
  }, [enabled, ref])

  return isOnScreen
}

/** useHasFinePointer reports whether the device has a hovering fine pointer. */
function useHasFinePointer(enabled: boolean): boolean {
  const [hasMouse, setHasMouse] = useState(false)

  useEffect(() => {
    if (!enabled) return

    const query = window.matchMedia('(hover: hover) and (pointer: fine)')
    const update = () =>
      setHasMouse((prev) => (prev === query.matches ? prev : query.matches))
    update()
    query.addEventListener('change', update)

    return () => {
      query.removeEventListener('change', update)
    }
  }, [enabled])

  return hasMouse
}

/** useElementRect tracks the bounding rect of the element while enabled. */
function useElementRect(
  ref: RefObject<HTMLElement | null>,
  enabled: boolean,
): DOMRect | null {
  const [elementRect, setElementRect] = useState<DOMRect | null>(null)

  useEffect(() => {
    const el = ref.current
    if (!enabled || !el) return

    const updateRect = () => {
      const next = el.getBoundingClientRect()
      setElementRect((prev) => (rectEquals(prev, next) ? prev : next))
    }

    updateRect()

    const resizeObserver = new ResizeObserver(updateRect)
    resizeObserver.observe(el)

    return () => {
      resizeObserver.disconnect()
    }
  }, [enabled, ref])

  return elementRect
}

interface LogoTilt {
  rotateX: number
  rotateY: number
  scale: number
}

const neutralTilt: LogoTilt = { rotateX: 0, rotateY: 0, scale: 1 }

/** useLogoTilt turns the pointer position relative to the logo into a 3D tilt. */
function useLogoTilt(
  mouse: { x: number | null; y: number | null },
  canAnimate: boolean,
  elementRect: DOMRect | null,
): LogoTilt {
  return useMemo(() => {
    if (!canAnimate || !elementRect) return neutralTilt

    const mouseX = mouse.x ?? 0
    const mouseY = mouse.y ?? 0
    const x = (mouseX / window.innerWidth) * 2 - 1
    const y = (mouseY / window.innerHeight) * 2 - 1

    const dx = mouseX - (elementRect.left + elementRect.width / 2)
    const dy = mouseY - (elementRect.top + elementRect.height / 2)
    const distance = Math.min(Math.hypot(dx, dy) / 1000, 1)

    return {
      rotateX: y * -9.262, // 8.42 * 1.1
      rotateY: x * 8.42,
      scale: 1 + distance * 0.002,
    }
  }, [mouse.x, mouse.y, canAnimate, elementRect])
}

const AnimatedLogo = ({
  className,
  containerClassName,
  followMouse = true,
  fixedSize,
  reduceMotion = false,
}: {
  className?: string
  containerClassName?: string
  followMouse?: boolean
  fixedSize?: string | number
  reduceMotion?: boolean
}) => {
  const [mouse, mouseRef] = useMouse<HTMLDivElement>()
  const docVisible = useDocumentVisibility()
  const isTabActive = useIsTabActive()
  const tracking = followMouse && !reduceMotion
  const isOnScreen = useIsOnScreen(mouseRef, tracking)
  const hasMouse = useHasFinePointer(tracking)
  const canRunAnimation =
    !reduceMotion && isOnScreen && docVisible === 'visible' && isTabActive
  const canAnimate = !reduceMotion && followMouse && canRunAnimation && hasMouse
  const elementRect = useElementRect(mouseRef, canAnimate)

  const transform = useLogoTilt(mouse, canAnimate, elementRect)

  return (
    <div
      ref={mouseRef}
      className={cn(
        'group relative perspective-logo',
        fixedSize && 'animated-logo-fixed-size',
        containerClassName,
      )}
      style={
        fixedSize
          ? {
              '--animated-logo-size':
                typeof fixedSize === 'number' ? `${fixedSize}px` : fixedSize,
            }
          : undefined
      }
    >
      <div
        className="animated-logo-transform relative size-20 @lg:h-28 @lg:w-28"
        style={{
          '--animated-logo-transform': `rotateX(${transform.rotateX}deg) rotateY(${transform.rotateY}deg) scale(${transform.scale})`,
          '--animated-logo-transition': reduceMotion
            ? 'none'
            : 'transform 0.8s ease-out',
        }}
      >
        {/* Background Gradient Layer - Blur for Depth */}
        <div
          className={cn(
            'absolute -inset-0.5 z-1 rounded-3xl opacity-50 blur-md transition duration-800 will-change-transform group-hover:scale-105 group-hover:opacity-55',
            'animated-logo-background',
            canRunAnimation
              ? 'animated-logo-blur'
              : 'animated-logo-blur-paused',
          )}
        />

        {/* Background Gradient Layer - Sharp for Clean Border */}
        <div
          className={cn(
            'absolute -inset-0.25 z-2 rounded-3xl will-change-transform',
            'animated-logo-background',
          )}
        />

        {/* Logo Image - Ensures it stays on top */}
        <div
          className={cn(
            'relative z-10 h-full w-full overflow-hidden rounded-3xl',
            fixedSize && 'animated-logo-fixed-content',
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

import * as ownerStylex from '@stylexjs/stylex'
import { groupMarker as ownergroupMarker } from '@s4wave/web/style/relations.stylex.js'
import { useMemo, useEffect, useState, type RefObject } from 'react'
import { useDocumentVisibility } from '@aptre/bldr-react'
import { useMouse } from '@uidotdev/usehooks'

import { useIsTabActive } from '@s4wave/app/ShellTabContext.js'
import spacewaveIcon from '@s4wave/web/images/spacewave-icon.png'

import './AnimatedLogo.css'

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import { motion as themeMotion } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles2 = ownerStylex.create({
  rule1: { position: 'relative', perspective: 1000 },
  rule2: {
    position: 'relative',
    width: {
      default: `calc(${themeLayout['--spacing']} * 20)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 28)`,
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 20)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 28)`,
    },
  },
  rule3: {
    position: 'absolute',
    inset: `calc(${themeLayout['--spacing']} * -.5)`,
    zIndex: 1,
    borderRadius: 'var(--radius-3xl)',
    opacity: {
      default: 0.5,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]: 0.55,
      },
    },
    '--tw-blur': 'blur(var(--blur-md))',
    filter:
      'var(--tw-blur, ) var(--tw-brightness, ) var(--tw-contrast, ) var(--tw-grayscale, ) var(--tw-hue-rotate, ) var(--tw-invert, ) var(--tw-saturate, ) var(--tw-sepia, ) var(--tw-drop-shadow, )',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.8s',
    '--tw-duration': '.8s',
    willChange: 'transform',
    backgroundColor: 'transparent',
    backgroundImage:
      'radial-gradient(circle farthest-corner at 100% 0, var(--color-brand), transparent), radial-gradient(circle farthest-corner at 0 100%, var(--color-logo-blue), transparent), radial-gradient(circle farthest-corner at 0 0, var(--color-brand), transparent), radial-gradient(circle at 50% 50%, var(--color-logo-base) 10%, var(--color-logo-dark) 80%)',
    backgroundPositionX: '0%',
    backgroundPositionY: '0%',
    backgroundSize: 'auto',
    backgroundRepeat: 'repeat',
    backgroundAttachment: 'scroll',
    backgroundOrigin: 'padding-box',
    backgroundClip: 'border-box',
    '--tw-scale-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]: '105%',
      },
    },
    '--tw-scale-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]: '105%',
      },
    },
    '--tw-scale-z': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]: '105%',
      },
    },
    scale: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          'var(--tw-scale-x) var(--tw-scale-y)',
      },
    },
  },
  rule4: {
    position: 'relative',
    zIndex: 10,
    height: '100%',
    width: '100%',
    overflow: 'hidden',
    borderRadius: 'var(--radius-3xl)',
  },
})

const stylexStyles = ownerStylex.create({
  rule1: {
    position: 'absolute',
    inset: `calc(${themeLayout['--spacing']} * -.25)`,
    zIndex: 2,
    borderRadius: 'var(--radius-3xl)',
    willChange: 'transform',
    backgroundColor: 'transparent',
    backgroundImage:
      'radial-gradient(circle farthest-corner at 100% 0, var(--color-brand), transparent), radial-gradient(circle farthest-corner at 0 100%, var(--color-logo-blue), transparent), radial-gradient(circle farthest-corner at 0 0, var(--color-brand), transparent), radial-gradient(circle at 50% 50%, var(--color-logo-base) 10%, var(--color-logo-dark) 80%)',
    backgroundPositionX: '0%',
    backgroundPositionY: '0%',
    backgroundSize: 'auto',
    backgroundRepeat: 'repeat',
    backgroundAttachment: 'scroll',
    backgroundOrigin: 'padding-box',
    backgroundClip: 'border-box',
  },
  rule2: { height: '100%', width: '100%', maxWidth: 'none' },
})

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
  sx,
  containerSx,
  followMouse = true,
  fixedSize,
  reduceMotion = false,
}: {
  sx?: ownerStylex.StyleXStyles
  className?: never
  containerSx?: ownerStylex.StyleXStyles
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
      sx={[
        stylexStyles2.rule1,
        ownergroupMarker,
        !!fixedSize && logoNative.fixed,
        containerSx,
      ]}
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
        sx={[
          stylexStyles2.rule2,
          logoNative.tilt,
          reduceMotion && logoNative.still,
        ]}
        style={{
          '--animated-logo-transform': `rotateX(${transform.rotateX}deg) rotateY(${transform.rotateY}deg) scale(${transform.scale})`,
        }}
      >
        {/* Background Gradient Layer - Blur for Depth */}
        <div
          sx={stylexStyles2.rule3}
          className={
            canRunAnimation ? 'animated-logo-blur' : 'animated-logo-blur-paused'
          }
        />

        {/* Background Gradient Layer - Sharp for Clean Border */}
        <div sx={[stylexStyles.rule1]} />

        {/* Logo Image - Ensures it stays on top */}
        <div sx={[stylexStyles2.rule4, sx]}>
          <img
            src={spacewaveIcon}
            alt="Spacewave Icon"
            sx={stylexStyles.rule2}
          />
        </div>
      </div>
    </div>
  )
}

export default AnimatedLogo

// logoNative owns the fixed dimensions and dynamic tilt without utility strings.
const logoNative = ownerStylex.create({
  fixed: {
    width: 'var(--animated-logo-size)',
    height: 'var(--animated-logo-size)',
  },
  tilt: {
    transform: 'var(--animated-logo-transform)',
    transitionProperty: 'transform',
    transitionDuration: '0.8s',
    transitionTimingFunction: 'ease-out',
    transitionDelay: '0s',
  },
  still: { transitionProperty: 'none' },
})

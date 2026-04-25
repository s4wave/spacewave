import { useEffect, useRef, useState } from 'react'
import { useDocumentVisibility } from '@aptre/bldr-react'

import { useIsTabActive } from '@s4wave/web/contexts/TabActiveContext.js'
import { cn } from '@s4wave/web/style/utils.js'

interface ShootingStar {
  id: number
  x: number
  y: number
  angle: number
  scale: number
  speed: number
  distance: number
}

interface ShootingStarsProps {
  minSpeed?: number
  maxSpeed?: number
  minDelay?: number
  maxDelay?: number
  starColor?: string
  trailColor?: string
  starWidth?: number
  starHeight?: number
  className?: string
  maxStars?: number
}

const getRandomStartPoint = () => {
  const side = Math.floor(Math.random() * 4)
  const offset = Math.random() * window.innerWidth
  switch (side) {
    case 0:
      return { x: offset, y: 0, angle: 45 }
    case 1:
      return { x: window.innerWidth, y: offset, angle: 135 }
    case 2:
      return { x: offset, y: window.innerHeight, angle: 225 }
    default:
      return { x: 0, y: offset, angle: 315 }
  }
}

export function ShootingStars({
  minSpeed = 8,
  maxSpeed = 18,
  minDelay = 1200,
  maxDelay = 4200,
  starColor = '#9E00FF',
  trailColor = '#2EB9DF',
  starWidth = 10,
  starHeight = 1,
  className,
  maxStars = 20,
}: ShootingStarsProps) {
  const [stars, setStars] = useState<ShootingStar[]>([])
  const docVisible = useDocumentVisibility()
  const isTabActive = useIsTabActive()
  const startRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const starIdCounter = useRef(0)
  const isActive = docVisible === 'visible' && isTabActive

  useEffect(() => {
    if (!isActive) {
      return
    }

    let animationFrame = 0
    let disposed = false

    function createStar() {
      if (disposed) {
        return
      }

      setStars((prevStars) => {
        if (prevStars.length >= maxStars) return prevStars

        const { x, y, angle } = getRandomStartPoint()
        const newStar: ShootingStar = {
          id: starIdCounter.current++,
          x,
          y,
          angle,
          scale: 1,
          speed: Math.random() * (maxSpeed - minSpeed) + minSpeed,
          distance: 0,
        }
        return [...prevStars, newStar]
      })

      const randomDelay = Math.random() * (maxDelay - minDelay) + minDelay
      timeoutRef.current = setTimeout(createStar, randomDelay)
    }

    function moveStars() {
      if (disposed) {
        return
      }

      setStars(
        (prevStars) =>
          prevStars
            .map((star) => {
              const newX =
                star.x + star.speed * Math.cos((star.angle * Math.PI) / 180)
              const newY =
                star.y + star.speed * Math.sin((star.angle * Math.PI) / 180)
              const newDistance = star.distance + star.speed
              const newScale = 1 + newDistance / 100

              if (
                newX < -20 ||
                newX > window.innerWidth + 20 ||
                newY < -20 ||
                newY > window.innerHeight + 20
              ) {
                return null
              }

              return {
                ...star,
                x: newX,
                y: newY,
                distance: newDistance,
                scale: newScale,
              }
            })
            .filter(Boolean) as ShootingStar[],
      )

      animationFrame = requestAnimationFrame(moveStars)
    }

    startRef.current = setTimeout(() => {
      if (disposed) {
        return
      }

      createStar()
      animationFrame = requestAnimationFrame(moveStars)
    }, 0)

    return () => {
      disposed = true
      cancelAnimationFrame(animationFrame)
      if (startRef.current) {
        clearTimeout(startRef.current)
        startRef.current = null
      }
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current)
        timeoutRef.current = null
      }
    }
  }, [isActive, minSpeed, maxSpeed, minDelay, maxDelay, maxStars])

  return (
    <svg
      className={cn('absolute inset-0 h-full w-full select-none', className)}
    >
      {(isActive ? stars : []).map((star) => (
        <rect
          key={star.id}
          x={star.x}
          y={star.y}
          width={starWidth * star.scale}
          height={starHeight}
          fill="url(#gradient)"
          transform={`rotate(${star.angle}, ${star.x + (starWidth * star.scale) / 2}, ${star.y + starHeight / 2})`}
        />
      ))}
      <defs>
        <linearGradient id="gradient" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" style={{ stopColor: trailColor, stopOpacity: 0 }} />
          <stop
            offset="100%"
            style={{ stopColor: starColor, stopOpacity: 1 }}
          />
        </linearGradient>
      </defs>
    </svg>
  )
}

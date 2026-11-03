import { keyframeNames } from '../../../web/style/tokens.stylex.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { LuCircleX, LuKeyboard, LuRotateCcw } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'

import { Keycap } from './Keycap.js'
import { chordFromKeyboardEvent } from './keyboard-utils.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'inline-flex',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    animationDelay: '0s',
    animationDirection: 'normal',
    animationDuration: '2s',
    animationFillMode: 'none',
    animationIterationCount: 'infinite',
    animationName: keyframeNames['pulse'],
    animationPlayState: 'running',
    animationTimeline: 'auto',
    animationTimingFunction: 'cubic-bezier(.4, 0, .6, 1)',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    justifyContent: 'center',
    minWidth: `calc(${themeLayout['--spacing']} * 28)`,
  },
})

interface BindingCaptureProps {
  className?: never
  binding: string
  commandLabel: string
  isCustomized?: boolean
  sx?: ComponentStyles
  onChange: (binding: string) => void
  onReset?: () => void
}

export function BindingCapture({
  binding,
  commandLabel,
  isCustomized = false,
  sx,
  onChange,
  onReset,
}: Omit<BindingCaptureProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  const captureButtonRef = useRef<HTMLButtonElement>(null)
  const [capturing, setCapturing] = useState(false)

  const startCapture = useCallback(() => {
    setCapturing(true)
  }, [])

  const cancelCapture = useCallback(() => {
    setCapturing(false)
  }, [])

  const clearBinding = useCallback(() => {
    onChange('')
    setCapturing(false)
  }, [onChange])

  const handleCaptureKeyDown = useEffectEvent((event: KeyboardEvent) => {
    if (document.activeElement !== captureButtonRef.current) {
      setCapturing(false)
      return
    }

    event.preventDefault()
    event.stopPropagation()

    if (event.key === 'Escape') {
      setCapturing(false)
      return
    }
    if (event.repeat) return

    const chord = chordFromKeyboardEvent(event)
    if (!chord) return

    onChange(chord)
    setCapturing(false)
  })

  useEffect(() => {
    if (!capturing) return

    window.addEventListener('keydown', handleCaptureKeyDown, { capture: true })
    return () =>
      window.removeEventListener('keydown', handleCaptureKeyDown, {
        capture: true,
      })
  }, [capturing])

  return (
    <div sx={[stylexStyles3.rule1, sx]}>
      <Button
        ref={captureButtonRef}
        type="button"
        aria-label={
          capturing
            ? `Recording shortcut for ${commandLabel}`
            : `Change shortcut for ${commandLabel}`
        }
        aria-pressed={capturing}
        variant="capture"
        data-capturing={capturing}
        sx={stylexStyles.rule1}
        onBlur={cancelCapture}
        onClick={capturing ? cancelCapture : startCapture}
      >
        {capturing ? (
          <>
            <LuKeyboard sx={stylexStyles2.rule1} />
            Press shortcut
          </>
        ) : (
          <Keycap chord={binding} />
        )}
      </Button>
      {binding ? (
        <Button
          type="button"
          variant="muted"
          size="iconSm"
          aria-label={`Clear shortcut for ${commandLabel}`}
          onClick={clearBinding}
        >
          <LuCircleX />
        </Button>
      ) : null}
      {isCustomized && onReset ? (
        <Button
          type="button"
          variant="muted"
          size="iconSm"
          aria-label={`Reset shortcut for ${commandLabel}`}
          onClick={onReset}
        >
          <LuRotateCcw />
        </Button>
      ) : null}
    </div>
  )
}

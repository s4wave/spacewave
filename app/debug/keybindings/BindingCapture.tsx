import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { LuCircleX, LuKeyboard, LuRotateCcw } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'

import { Keycap } from './Keycap.js'
import { chordFromKeyboardEvent } from './keyboard-utils.js'

interface BindingCaptureProps {
  binding: string
  commandLabel: string
  isCustomized?: boolean
  className?: string
  onChange: (binding: string) => void
  onReset?: () => void
}

export function BindingCapture({
  binding,
  commandLabel,
  isCustomized = false,
  className,
  onChange,
  onReset,
}: BindingCaptureProps) {
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
    <div className={cn('inline-flex items-center gap-1.5', className)}>
      <Button
        ref={captureButtonRef}
        type="button"
        variant="outline"
        size="sm"
        aria-label={
          capturing
            ? `Recording shortcut for ${commandLabel}`
            : `Change shortcut for ${commandLabel}`
        }
        aria-pressed={capturing}
        className={cn(
          'border-foreground/10 bg-background/40 hover:border-foreground/20 min-w-28 justify-center font-normal',
          capturing &&
            'border-brand/50 bg-brand/10 text-brand ring-brand/20 ring-4',
        )}
        onBlur={cancelCapture}
        onClick={capturing ? cancelCapture : startCapture}
      >
        {capturing ? (
          <>
            <LuKeyboard className="animate-pulse" />
            Press shortcut
          </>
        ) : (
          <Keycap chord={binding} />
        )}
      </Button>
      {binding ? (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="text-foreground-alt/55 hover:text-foreground size-8"
          aria-label={`Clear shortcut for ${commandLabel}`}
          onClick={clearBinding}
        >
          <LuCircleX />
        </Button>
      ) : null}
      {isCustomized && onReset ? (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="text-foreground-alt/55 hover:text-foreground size-8"
          aria-label={`Reset shortcut for ${commandLabel}`}
          onClick={onReset}
        >
          <LuRotateCcw />
        </Button>
      ) : null}
    </div>
  )
}

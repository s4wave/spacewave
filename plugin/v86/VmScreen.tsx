import { useCallback, useEffect, useRef, useState } from 'react'
import type { KeyEvent } from '@aptre/v86'

import { cn } from '@s4wave/web/style/utils.js'

import { attachScreenView, type ScreenView } from './screen-channel.js'

// VmScreenProps configures the screen of one VM.
export interface VmScreenProps {
  // objectKey is the VmV86 whose VGA screen and keyboard the view attaches to.
  objectKey: string
  // hidden keeps the screen mounted but not shown.
  hidden?: boolean
}

// VmScreen draws the VM's VGA screen scaled to fit. Clicking the screen
// captures the keyboard for the guest until Ctrl+Alt+G or focus leaves.
export function VmScreen({ objectKey, hidden }: VmScreenProps) {
  const displayRef = useRef<HTMLDivElement | null>(null)
  const viewRef = useRef<ScreenView | null>(null)
  const [captured, setCaptured] = useState(false)
  const [loadError, setLoadError] = useState('')

  useEffect(() => {
    const display = displayRef.current
    if (!display || !objectKey) return

    // Load the screen adapter on first use and attach it to the runner.
    let closed = false
    import('@aptre/v86/screen')
      .then(({ ScreenAdapter }) => {
        if (closed) return
        viewRef.current = attachScreenView(ScreenAdapter, display, objectKey)
      })
      .catch((err: unknown) => {
        if (!closed)
          setLoadError(err instanceof Error ? err.message : String(err))
      })

    return () => {
      closed = true
      viewRef.current?.close()
      viewRef.current = null
      display.replaceChildren()
    }
  }, [objectKey])

  // release returns the keyboard to the page and lifts every guest key.
  const release = useCallback(() => {
    setCaptured(false)
    viewRef.current?.release()
  }, [])

  // handleKey sends captured keys to the guest. Ctrl+Alt+G releases.
  const handleKey = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>, down: boolean) => {
      if (!captured) return
      e.preventDefault()
      e.stopPropagation()
      if (down && e.ctrlKey && e.altKey && e.code === 'KeyG') {
        release()
        return
      }
      viewRef.current?.key(toKeyEvent(e), down)
    },
    [captured, release],
  )

  return (
    <div
      role="application"
      aria-label="Virtual machine screen"
      tabIndex={0}
      onClick={(e) => {
        e.currentTarget.focus()
        setCaptured(true)
      }}
      onKeyDown={(e) => handleKey(e, true)}
      onKeyUp={(e) => handleKey(e, false)}
      onBlur={() => {
        if (captured) release()
      }}
      className={cn(
        'relative min-h-0 flex-1 cursor-text overflow-hidden bg-black outline-none',
        'focus-visible:ring-brand/40 focus-visible:ring-2 focus-visible:ring-inset',
        hidden && 'hidden',
      )}
    >
      <div
        ref={displayRef}
        className="flex size-full items-center justify-center [&>canvas]:size-full! [&>canvas]:object-contain [&>div]:hidden!"
      />
      <div
        className={cn(
          'pointer-events-none absolute top-2 right-2 rounded-md border px-2 py-1 text-xs backdrop-blur-sm transition-colors',
          captured
            ? 'border-brand/40 bg-brand/15 text-brand'
            : 'border-foreground/10 bg-background/60 text-foreground-alt',
        )}
        aria-live="polite"
      >
        {loadError
          ? `The screen failed to load: ${loadError}`
          : captured
            ? 'Keyboard captured. Ctrl+Alt+G releases.'
            : 'Click to type into the VM'}
      </div>
    </div>
  )
}

// toKeyEvent keeps the fields of a key event the v86 keyboard reads.
function toKeyEvent(e: React.KeyboardEvent): KeyEvent {
  return {
    code: e.code,
    key: e.key,
    keyCode: e.keyCode,
    altKey: e.altKey,
    shiftKey: e.shiftKey,
    ctrlKey: e.ctrlKey,
    repeat: e.repeat,
  }
}

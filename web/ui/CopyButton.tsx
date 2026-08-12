import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type MouseEvent,
} from 'react'
import { LuCheck, LuCopy, LuTriangleAlert } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

// CopyButtonProps configures the CopyButton.
export interface CopyButtonProps {
  // text is the string copied to the clipboard when the button is pressed.
  text: string
  // className is appended to the rendered button's class list.
  className?: string
  // label overrides the default aria-label used in the idle state.
  label?: string
  // size selects between the small and medium icon sizes.
  size?: 'sm' | 'md'
}

// COPIED_RESET_MS is how long the "Copied" affordance stays visible.
const COPIED_RESET_MS = 2000

// CopyButton renders a small button that copies text to the clipboard
// and shows a transient "Copied" affordance. It is the shared primitive
// for inline copy buttons across the app.
export function CopyButton({
  text,
  className,
  label = 'Copy',
  size = 'sm',
}: CopyButtonProps) {
  const [status, setStatus] = useState<'idle' | 'copied' | 'error'>('idle')
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const generationRef = useRef(0)
  const mountedRef = useRef(true)

  const handleCopy = useCallback(
    async (event: MouseEvent<HTMLButtonElement>) => {
      event.preventDefault()
      event.stopPropagation()
      const generation = ++generationRef.current
      if (timerRef.current != null) {
        clearTimeout(timerRef.current)
      }
      try {
        await navigator.clipboard.writeText(text)
        if (!mountedRef.current || generation !== generationRef.current) return
        setStatus('copied')
      } catch {
        if (!mountedRef.current || generation !== generationRef.current) return
        setStatus('error')
      }
      timerRef.current = setTimeout(() => {
        if (!mountedRef.current || generation !== generationRef.current) return
        setStatus('idle')
        timerRef.current = null
      }, COPIED_RESET_MS)
    },
    [text],
  )

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      generationRef.current += 1
      if (timerRef.current != null) {
        clearTimeout(timerRef.current)
        timerRef.current = null
      }
    }
  }, [])

  const iconCls = size === 'md' ? 'h-4 w-4' : 'h-3.5 w-3.5'
  const boxCls = size === 'md' ? 'h-7 w-7' : 'h-6 w-6'

  return (
    <button
      type="button"
      onClick={handleCopy}
      onMouseDown={(event) => event.stopPropagation()}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') event.stopPropagation()
      }}
      aria-label={
        status === 'copied'
          ? 'Copied'
          : status === 'error'
            ? 'Copy failed'
            : label
      }
      className={cn(
        'text-foreground-alt hover:text-foreground flex shrink-0 items-center justify-center rounded transition-colors',
        boxCls,
        className,
      )}
    >
      <span className="sr-only" role="status" aria-live="polite">
        {status === 'copied'
          ? 'Copied'
          : status === 'error'
            ? 'Copy failed'
            : ''}
      </span>
      {status === 'copied' ? (
        <LuCheck className={cn(iconCls, 'text-emerald-500')} />
      ) : status === 'error' ? (
        <LuTriangleAlert className={cn(iconCls, 'text-destructive')} />
      ) : (
        <LuCopy className={iconCls} />
      )}
    </button>
  )
}

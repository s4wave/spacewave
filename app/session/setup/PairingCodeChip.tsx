import { useCallback, useState } from 'react'
import { LuCheck } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

interface PairingCodeChipProps {
  code: string
  className?: string
}

// PairingCodeChip renders an 8-character pairing code as a centered, copyable
// chip matching the code-entry field. The copied indicator clears after a short
// delay.
export function PairingCodeChip({ code, className }: PairingCodeChipProps) {
  const [copied, setCopied] = useState(false)
  const formatted =
    code.length > 4 ? `${code.slice(0, 4)} ${code.slice(4)}` : code

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(code).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    })
  }, [code])

  return (
    <div className={cn('flex flex-col items-center gap-2', className)}>
      <button
        type="button"
        onClick={handleCopy}
        aria-label="Copy pairing code"
        className={cn(
          'border-foreground/20 bg-foreground/5 text-foreground rounded-md border font-mono text-2xl font-bold tracking-[0.2em]',
          'flex h-14 items-center justify-center px-4 select-all',
          'cursor-pointer transition-colors hover:border-foreground/40',
          'focus-visible:border-brand/50 focus-visible:outline-none',
        )}
      >
        {formatted}
      </button>
      <span
        className={cn('text-xs', copied ? 'text-brand' : 'text-foreground-alt')}
        aria-live="polite"
      >
        {copied ? (
          <>
            <LuCheck className="mr-1 inline size-3" aria-hidden="true" />
            Copied
          </>
        ) : (
          'Click the code to copy'
        )}
      </span>
    </div>
  )
}

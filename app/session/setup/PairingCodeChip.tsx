import { useCallback, useState } from 'react'
import { LuCheck, LuCopy } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

interface PairingCodeChipProps {
  code: string
  className?: string
}

// PairingCodeChip renders an 8-character pairing code as a grouped monospace
// chip matching the code-entry field, with a copy-to-clipboard control. The
// copied indicator clears after a short delay.
export function PairingCodeChip({ code, className }: PairingCodeChipProps) {
  const [copied, setCopied] = useState(false)
  const formatted =
    code.length > 4 ? `${code.slice(0, 4)} ${code.slice(4)}` : code

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(code)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }, [code])

  return (
    <div className={cn('flex items-center gap-2', className)}>
      <span
        className={cn(
          'border-foreground/20 bg-foreground/5 text-foreground rounded-md border font-mono text-2xl font-bold tracking-[0.2em]',
          'flex h-14 items-center justify-center px-4 select-all',
        )}
      >
        {formatted}
      </span>
      <button
        type="button"
        onClick={handleCopy}
        title="Copy pairing code"
        aria-label="Copy pairing code"
        className={cn(
          'rounded-md border transition-all duration-300',
          'border-foreground/20 hover:border-foreground/40',
          'flex size-10 shrink-0 items-center justify-center',
        )}
      >
        {copied ? (
          <LuCheck className="text-brand size-4" />
        ) : (
          <LuCopy className="text-foreground-alt size-4" />
        )}
      </button>
    </div>
  )
}

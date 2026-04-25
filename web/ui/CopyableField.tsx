import React, { useCallback, useState } from 'react'
import { LuCheck, LuCopy } from 'react-icons/lu'

// CopyableField displays a labeled value that can be copied to clipboard.
export interface CopyableFieldProps {
  label: string
  value: string
}

export function CopyableField({ label, value }: CopyableFieldProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }, [value])

  return (
    <div className="flex flex-col gap-1">
      <span className="text-foreground-alt text-xs select-none">{label}</span>
      <button
        onClick={handleCopy}
        className="group hover:bg-foreground/5 -ml-2 flex items-center gap-2 rounded-md px-2 py-1 text-left transition-colors"
        aria-label={copied ? 'Copied!' : 'Click to copy'}
      >
        <span className="text-foreground font-mono text-xs break-all">
          {value}
        </span>
        {copied ?
          <LuCheck className="h-3.5 w-3.5 flex-shrink-0 text-green-500" />
        : <LuCopy className="h-3.5 w-3.5 flex-shrink-0 opacity-0 transition-opacity group-hover:opacity-100" />
        }
      </button>
    </div>
  )
}

import { useCallback, useEffect, useRef, useState } from 'react'
import { LuCheck, LuCopy } from 'react-icons/lu'

import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { cn } from '@s4wave/web/style/utils.js'

import { highlightCode } from './highlight.js'

// CodeBlockProps are the props for CodeBlock.
export interface CodeBlockProps {
  code: string
  language?: string
  className?: string
}

/**
 * CodeBlock renders highlighted code with a copy button. It shows plain text
 * while the grammar loads and when Shiki does not know the language.
 */
export function CodeBlock({ code, language, className }: CodeBlockProps) {
  const { data: html } = usePromise(
    useCallback(
      () => highlightCode(code, language || 'text'),
      [code, language],
    ),
  )

  const [copied, setCopied] = useState(false)
  const copyTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => {
    return () => clearTimeout(copyTimer.current)
  }, [])

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(code)
    setCopied(true)
    clearTimeout(copyTimer.current)
    copyTimer.current = setTimeout(() => setCopied(false), 1500)
  }, [code])

  return (
    <div className={cn('group/code relative', className)}>
      <button
        type="button"
        onClick={handleCopy}
        className={cn(
          'absolute top-2.5 right-2.5 z-10 flex size-7 items-center justify-center rounded-md transition-all',
          'opacity-0 group-hover/code:opacity-100',
          copied
            ? 'bg-brand/20 text-brand'
            : 'bg-foreground/5 text-foreground-alt/40 hover:bg-foreground/10 hover:text-foreground-alt',
        )}
        title="Copy code"
      >
        {copied ? (
          <LuCheck className="size-3.5" />
        ) : (
          <LuCopy className="size-3.5" />
        )}
      </button>

      {html ? (
        <div dangerouslySetInnerHTML={{ __html: html }} />
      ) : (
        <pre>
          <code>{code}</code>
        </pre>
      )}
    </div>
  )
}

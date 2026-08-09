import { useCallback, useEffect, useRef, useState } from 'react'
import { LuCheck, LuCopy } from 'react-icons/lu'
import type { Highlighter } from 'shiki'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { cn } from '@s4wave/web/style/utils.js'

let highlighterPromise: Promise<Highlighter> | null = null

function getHighlighter(): Promise<Highlighter> {
  highlighterPromise ??= import('shiki').then((shiki) =>
    shiki.createHighlighter({
      themes: ['vesper'],
      langs: [
        'typescript',
        'javascript',
        'go',
        'bash',
        'json',
        'yaml',
        'html',
        'css',
        'markdown',
        'proto',
        'toml',
        'shell',
        'tsx',
        'jsx',
      ],
    }),
  )
  return highlighterPromise
}

interface CodeBlockProps {
  lang: string
  code: string
}

// CodeBlock renders syntax-highlighted code and keeps plain text visible while
// the highlighter Resource is pending or unavailable.
export function CodeBlock({ lang, code }: CodeBlockProps) {
  const highlighted = useResource(
    async (signal) => {
      const highlighter = await getHighlighter()
      if (signal.aborted) return null
      const language = highlighter.getLoadedLanguages().includes(lang)
        ? lang
        : 'text'
      return highlighter.codeToHtml(code.replace(/\n$/, ''), {
        lang: language,
        theme: 'vesper',
      })
    },
    [code, lang],
  )
  const [copied, setCopied] = useState(false)
  const copyTimer = useRef<ReturnType<typeof setTimeout>>(undefined)

  useEffect(() => () => clearTimeout(copyTimer.current), [])

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(code.replace(/\n$/, ''))
    setCopied(true)
    clearTimeout(copyTimer.current)
    copyTimer.current = setTimeout(() => setCopied(false), 1500)
  }, [code])

  return (
    <div className="group/code relative">
      <button
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
      {highlighted.value ? (
        <div dangerouslySetInnerHTML={{ __html: highlighted.value }} />
      ) : (
        <pre>
          <code>{code}</code>
        </pre>
      )}
    </div>
  )
}

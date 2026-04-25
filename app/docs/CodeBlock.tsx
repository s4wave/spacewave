import { useState, useEffect, useCallback, useRef } from 'react'
import { LuCopy, LuCheck } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import type { Highlighter } from 'shiki'

// highlighterPromise is a lazy singleton for the Shiki highlighter instance.
let highlighterPromise: Promise<Highlighter> | null = null

// getHighlighter returns the shared Shiki highlighter, creating it on first call.
function getHighlighter(): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = import('shiki').then((shiki) =>
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
  }
  return highlighterPromise
}

// CodeBlockProps defines the props for CodeBlock.
interface CodeBlockProps {
  lang: string
  code: string
}

// CodeBlock renders syntax-highlighted code using Shiki with vitesse-dark theme.
export function CodeBlock({ lang, code }: CodeBlockProps) {
  const [html, setHtml] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    let cancelled = false
    getHighlighter().then((highlighter) => {
      if (cancelled) return
      const trimmed = code.replace(/\n$/, '')
      const language =
        highlighter.getLoadedLanguages().includes(lang) ? lang : 'text'
      setHtml(
        highlighter.codeToHtml(trimmed, {
          lang: language,
          theme: 'vesper',
        }),
      )
    })
    return () => {
      cancelled = true
    }
  }, [code, lang])

  const copyTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => {
    return () => clearTimeout(copyTimer.current)
  }, [])

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(code.replace(/\n$/, ''))
    setCopied(true)
    clearTimeout(copyTimer.current)
    copyTimer.current = setTimeout(() => setCopied(false), 1500)
  }, [code])

  return (
    <div className="group/code relative">
      <button
        onClick={handleCopy}
        className={cn(
          'absolute top-2.5 right-2.5 z-10 flex h-7 w-7 items-center justify-center rounded-md transition-all',
          'opacity-0 group-hover/code:opacity-100',
          copied ?
            'bg-brand/20 text-brand'
          : 'bg-foreground/5 text-foreground-alt/40 hover:bg-foreground/10 hover:text-foreground-alt',
        )}
        title="Copy code"
      >
        {copied ?
          <LuCheck className="h-3.5 w-3.5" />
        : <LuCopy className="h-3.5 w-3.5" />}
      </button>
      {html ?
        <div dangerouslySetInnerHTML={{ __html: html }} />
      : <pre>
          <code>{code}</code>
        </pre>
      }
    </div>
  )
}

// PreBlock is the markdown-to-jsx override for <pre> elements.
// It detects fenced code blocks and routes them through CodeBlock.
export function PreBlock({
  children,
  ...props
}: React.HTMLAttributes<HTMLPreElement> & { children?: React.ReactNode }) {
  if (
    children &&
    typeof children === 'object' &&
    'props' in (children as React.ReactElement)
  ) {
    const child = children as React.ReactElement<{
      className?: string
      children?: React.ReactNode
    }>
    const className = child.props?.className || ''
    const langMatch = className.match(/(?:^|\s)(?:language-|lang-)(\S+)/)
    if (langMatch) {
      const lang = langMatch[1].replace(/^language-|^lang-/, '')
      const code =
        typeof child.props.children === 'string' ?
          child.props.children
        : String(child.props.children ?? '')
      return <CodeBlock lang={lang} code={code} />
    }
  }
  return <pre {...props}>{children}</pre>
}

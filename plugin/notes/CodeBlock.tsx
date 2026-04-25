import {
  isValidElement,
  type HTMLAttributes,
  type ReactElement,
  type ReactNode,
  useEffect,
  useMemo,
  useState,
} from 'react'
import type { BundledLanguage, Highlighter } from 'shiki'

let highlighterPromise: Promise<Highlighter> | null = null
let highlighterInstance: Highlighter | null = null

// getHighlighter returns a cached Shiki highlighter singleton.
async function getHighlighter(): Promise<Highlighter> {
  if (highlighterInstance) return highlighterInstance
  if (!highlighterPromise) {
    highlighterPromise = import('shiki').then((mod) =>
      mod.createHighlighter({
        themes: ['vesper'],
        langs: [],
      }),
    )
    void highlighterPromise.then((h) => {
      highlighterInstance = h
    })
  }
  return highlighterPromise
}

// CodeBlockProps defines the props for CodeBlock.
interface CodeBlockProps {
  code: string
  language?: string
  className?: string
}

// CodeBlock renders syntax-highlighted code using Shiki.
// Falls back to plain preformatted text while the highlighter loads.
export function CodeBlock({ code, language, className }: CodeBlockProps) {
  const [html, setHtml] = useState<string | null>(null)

  useEffect(() => {
    if (!code) return
    let cancelled = false
    const lang = language || 'text'

    void getHighlighter()
      .then(async (h) => {
        if (cancelled) return
        const loadedLangs = h.getLoadedLanguages()
        if (!loadedLangs.includes(lang as BundledLanguage)) {
          try {
            await h.loadLanguage(lang as BundledLanguage)
          } catch {
            // Language not supported, render as plain text.
            if (!cancelled) setHtml(null)
            return
          }
        }
        if (cancelled) return
        const result = h.codeToHtml(code, {
          lang,
          theme: 'vesper',
        })
        setHtml(result)
      })
      .catch(() => {
        if (!cancelled) setHtml(null)
      })

    return () => {
      cancelled = true
    }
  }, [code, language])

  if (html) {
    return (
      <div
        className={className}
        dangerouslySetInnerHTML={{ __html: html }}
      />
    )
  }

  return (
    <pre className={className}>
      <code>{code}</code>
    </pre>
  )
}

// markdownCodeOverrides returns markdown-to-jsx overrides for code blocks
// using Shiki syntax highlighting.
export function useMarkdownCodeOverrides() {
  return useMemo(
    () => ({
      overrides: {
        pre: {
          component: PreBlock,
        },
      },
    }),
    [],
  )
}

interface PreCodeProps {
  children?: string
  className?: string
}

interface PreBlockProps extends HTMLAttributes<HTMLPreElement> {
  children?: ReactNode
}

// PreBlock extracts the language and code from a fenced code block
// rendered by markdown-to-jsx and delegates to CodeBlock.
function PreBlock({ children, ...rest }: PreBlockProps) {
  if (isValidElement(children) && children.type === 'code') {
    const codeElement = children as ReactElement<PreCodeProps>
    const code = codeElement.props.children ?? ''
    const className = codeElement.props.className ?? ''
    const match = /language-(\w+)/.exec(className)
    const language = match?.[1]
    return <CodeBlock code={code} language={language} {...rest} />
  }
  return <pre {...rest}>{children}</pre>
}

import {
  isValidElement,
  type HTMLAttributes,
  type ReactElement,
  type ReactNode,
  useMemo,
} from 'react'
import type { BundledLanguage, Highlighter } from 'shiki'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'

let highlighterPromise: Promise<Highlighter> | null = null
let highlighterInstance: Highlighter | null = null

// getHighlighter returns a cached Shiki highlighter singleton.
async function getHighlighter(): Promise<Highlighter> {
  if (highlighterInstance) return highlighterInstance
  highlighterPromise ??= import('shiki').then((mod) =>
    mod.createHighlighter({
      themes: ['vesper'],
      langs: [],
    }),
  )
  highlighterInstance = await highlighterPromise
  return highlighterInstance
}

// CodeBlockProps defines the props for CodeBlock.
interface CodeBlockProps {
  code: string
  language?: string
  className?: string
}

// CodeBlock renders highlighted code and keeps plain text visible while its
// Resource loads or when Shiki does not support the requested language.
export function CodeBlock({ code, language, className }: CodeBlockProps) {
  const highlighted = useResource(
    async (signal) => {
      if (!code) return null
      const highlighter = await getHighlighter()
      if (signal.aborted) return null
      const lang = language || 'text'
      if (!highlighter.getLoadedLanguages().includes(lang)) {
        try {
          await highlighter.loadLanguage(lang as BundledLanguage)
        } catch {
          return null
        }
      }
      if (signal.aborted) return null
      return highlighter.codeToHtml(code, { lang, theme: 'vesper' })
    },
    [code, language],
  )

  if (highlighted.value) {
    return (
      <div
        className={className}
        dangerouslySetInnerHTML={{ __html: highlighted.value }}
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

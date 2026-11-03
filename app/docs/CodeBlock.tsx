import { useCallback, useEffect, useRef, useState } from 'react'
import { LuCheck, LuCopy } from '@s4wave/web/images/icons.js'

import { codeFenceKey, readCodeFence, type CodeFence } from './code-fence.js'
import highlights from './code-highlight.generated.json'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import { codeGroupMarker } from '../../web/style/relations.stylex.js'

import {
  radii as themeRadii,
  motion as themeMotion,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'
import { codeGroupMarker as codeGroupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    opacity: {
      default: 0,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', codeGroupMarker2)]: 1,
      },
    },
    position: 'absolute',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    zIndex: 10,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    right: `calc(${themeLayout['--spacing']} * 2.5)`,
    top: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule3: {
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      },
    },
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
  },
})

const stylexStyles2 = stylex.create({ rule1: { position: 'relative' } })

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// highlightedHtml maps each docs code fence to its build-time Shiki HTML.
// Regenerate with `bun run gen:docs-code` after editing docs code fences.
const highlightedHtml = new Map(
  highlights.map((fence) => [codeFenceKey(fence), fence.html]),
)

// getHighlightedHtml returns the prerendered HTML for a docs code fence.
export function getHighlightedHtml(fence: CodeFence): string | undefined {
  return highlightedHtml.get(codeFenceKey(fence))
}

// CodeBlock renders a docs code fence with its prerendered highlighting and a
// copy button. A fence missing from the generated table renders as plain text.
export function CodeBlock({ fence }: { fence: CodeFence }) {
  const [copied, setCopied] = useState(false)
  const copyTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => {
    return () => clearTimeout(copyTimer.current)
  }, [])

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(fence.code)
    setCopied(true)
    clearTimeout(copyTimer.current)
    copyTimer.current = setTimeout(() => setCopied(false), 1500)
  }, [fence.code])

  const html = getHighlightedHtml(fence)
  return (
    <div sx={[stylexStyles2.rule1, codeGroupMarker]}>
      <button
        type="button"
        onClick={handleCopy}
        sx={[
          stylexStyles3.rule1,
          copied ? stylexStyles3.rule2 : stylexStyles3.rule3,
        ]}
        title="Copy code"
      >
        {copied ? (
          <LuCheck sx={stylexStyles.rule1} />
        ) : (
          <LuCopy sx={stylexStyles.rule1} />
        )}
      </button>
      {html ? (
        <div dangerouslySetInnerHTML={{ __html: html }} />
      ) : (
        <pre>
          <code>{fence.code}</code>
        </pre>
      )}
    </div>
  )
}

// PreBlock is the markdown-to-jsx override for <pre> elements. It routes
// fenced code blocks through CodeBlock.
export function PreBlock({
  children,
  ...props
}: React.HTMLAttributes<HTMLPreElement>) {
  const fence = readCodeFence(children)
  if (fence) return <CodeBlock fence={fence} />
  return <pre {...props}>{children}</pre>
}

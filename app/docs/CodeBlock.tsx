import { useCallback, useEffect, useRef, useState } from 'react'
import { LuCheck, LuCopy } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import { codeFenceKey, readCodeFence, type CodeFence } from './code-fence.js'
import highlights from './code-highlight.generated.json'

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

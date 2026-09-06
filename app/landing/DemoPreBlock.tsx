import {
  isValidElement,
  useCallback,
  useEffect,
  useRef,
  useState,
  type HTMLAttributes,
} from 'react'
import { LuCheck, LuCopy } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import highlights from './demo-code.generated.json'

// DemoPreBlock uses generated highlights for fixed examples and plain text for edits.
// Only build-generated markup enters the HTML sink; edited code remains React text.
export function DemoPreBlock({
  children,
  ...props
}: HTMLAttributes<HTMLPreElement>) {
  const [copied, setCopied] = useState(false)
  const copyTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const child = isValidElement<{ className?: string; children?: string }>(
    children,
  )
    ? children
    : undefined
  const lang = child?.props.className?.match(
    /(?:^|\s)(?:language-|lang-)(\S+)/,
  )?.[1]
  const code =
    typeof child?.props.children === 'string'
      ? child.props.children.replace(/\n$/, '')
      : ''
  const highlighted = highlights.find(
    (item) => item.code === code && item.lang === lang,
  )

  useEffect(() => () => clearTimeout(copyTimer.current), [])

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(code)
    setCopied(true)
    clearTimeout(copyTimer.current)
    copyTimer.current = setTimeout(() => setCopied(false), 1500)
  }, [code])

  if (!lang) return <pre {...props}>{children}</pre>

  return (
    <div className="group/code relative">
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
      {highlighted ? (
        <div dangerouslySetInnerHTML={{ __html: highlighted.html }} />
      ) : (
        <pre>
          <code>{code}</code>
        </pre>
      )}
    </div>
  )
}

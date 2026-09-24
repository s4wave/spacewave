import { isValidElement, type HTMLAttributes, type ReactNode } from 'react'

import { CodeBlock } from './CodeBlock.js'

/**
 * CodePreBlock is the markdown-to-jsx override for <pre>. It renders a fenced
 * code block through CodeBlock and any other <pre> unchanged.
 */
export function CodePreBlock({
  children,
  ...props
}: HTMLAttributes<HTMLPreElement>) {
  if (
    !isValidElement<{ className?: string; children?: ReactNode }>(children) ||
    children.type !== 'code'
  ) {
    return <pre {...props}>{children}</pre>
  }

  const code =
    typeof children.props.children === 'string' ? children.props.children : ''
  const language = /language-(\S+)/.exec(children.props.className ?? '')?.[1]
  return <CodeBlock code={code.replace(/\n$/, '')} language={language} />
}

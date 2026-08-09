import type { HTMLAttributes, ReactElement, ReactNode } from 'react'

import { CodeBlock } from './CodeBlock.js'

type PreBlockProps = HTMLAttributes<HTMLPreElement> & { children?: ReactNode }

// PreBlock routes fenced markdown code through CodeBlock.
export function PreBlock({ children, ...props }: PreBlockProps) {
  if (children && typeof children === 'object' && 'props' in children) {
    const child = children as ReactElement<{
      className?: string
      children?: ReactNode
    }>
    const className = child.props?.className || ''
    const langMatch = className.match(/(?:^|\s)(?:language-|lang-)(\S+)/)
    if (langMatch) {
      const lang = langMatch[1].replace(/^language-|^lang-/, '')
      const code =
        typeof child.props.children === 'string' ? child.props.children : ''
      return <CodeBlock lang={lang} code={code} />
    }
  }
  return <pre {...props}>{children}</pre>
}

import { isValidElement, type ReactNode } from 'react'

// CodeFence is a fenced code block parsed by markdown-to-jsx.
export interface CodeFence {
  lang: string
  code: string
}

// readCodeFence returns the fenced code block rendered as the children of a
// markdown <pre>, or null when the <pre> holds no language-tagged code.
export function readCodeFence(children: ReactNode): CodeFence | null {
  if (!isValidElement<{ className?: string; children?: ReactNode }>(children)) {
    return null
  }
  const langMatch = (children.props.className ?? '').match(
    /(?:^|\s)(?:language-|lang-)(\S+)/,
  )
  if (!langMatch) return null
  const code =
    typeof children.props.children === 'string' ? children.props.children : ''
  return { lang: langMatch[1], code: code.replace(/\n$/, '') }
}

// codeFenceKey identifies a fence in the generated highlight table.
export function codeFenceKey({ lang, code }: CodeFence): string {
  return `${lang}\n${code}`
}

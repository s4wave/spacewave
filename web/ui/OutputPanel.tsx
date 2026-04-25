import type { ReactNode } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

// OutputLineRule defines a coloring rule for output lines.
export interface OutputLineRule {
  // match is either a prefix string to check or a function.
  match: string | ((line: string) => boolean)
  // className is the Tailwind classes to apply when matched.
  className: string
}

// OutputPanelProps are the props for the OutputPanel component.
export interface OutputPanelProps {
  // lines is the array of output lines to display.
  lines: string[]
  // rules is an optional array of line coloring rules.
  // Rules are evaluated in order; first match wins.
  rules?: OutputLineRule[]
  // placeholder is shown when lines is empty and there's no error.
  placeholder?: ReactNode
  // error is an optional error message to display at the top.
  error?: string | null
  // className is an optional CSS class for the container.
  className?: string
  // testId is an optional data-testid attribute.
  testId?: string
}

const defaultRules: OutputLineRule[] = [
  { match: 'PASS', className: 'text-success' },
  { match: 'FAIL', className: 'text-error' },
  { match: '[stderr]', className: 'text-warning' },
  { match: 'Running:', className: 'text-sm font-semibold' },
  { match: 'Error:', className: 'text-error font-medium' },
]

// OutputPanel displays scrollable log output with syntax-highlighted lines.
export function OutputPanel({
  lines,
  rules = defaultRules,
  placeholder = 'No output',
  error,
  className,
  testId,
}: OutputPanelProps) {
  const getLineClassName = (line: string): string => {
    for (const rule of rules) {
      const matches =
        typeof rule.match === 'string' ?
          line.startsWith(rule.match)
        : rule.match(line)
      if (matches) {
        return rule.className
      }
    }
    return 'text-text-secondary'
  }

  return (
    <div
      className={cn(
        'flex-1 overflow-auto rounded border p-2 font-mono text-xs',
        'bg-background-secondary text-foreground',
        'border-ui-outline',
        className,
      )}
      data-testid={testId}
    >
      {error && (
        <div className="text-error mb-2 font-medium">Error: {error}</div>
      )}
      {lines.length === 0 && !error && (
        <div className="text-foreground-alt">{placeholder}</div>
      )}
      {lines.map((line, i) => (
        <div key={i} className={cn('leading-relaxed', getLineClassName(line))}>
          {line}
        </div>
      ))}
    </div>
  )
}

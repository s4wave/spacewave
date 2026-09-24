import { cn } from '@s4wave/web/style/utils.js'

// Fact is one labeled reading in an inspector.
export interface Fact {
  label: string
  value: string | number | bigint | boolean | null | undefined
  mono?: boolean
  tone?: 'warning' | 'error'
}

// Facts renders labeled readings as a definition list. Readings with no value
// are omitted so the list shows only what the system reported; an empty list
// renders the fallback line instead.
export function Facts({
  facts,
  empty = 'Nothing reported.',
}: {
  facts: Fact[]
  empty?: string
}) {
  const present = facts.filter(
    (fact) => fact.value !== '' && fact.value != null,
  )
  if (!present.length) {
    return <p className="text-foreground-alt/50 text-xs">{empty}</p>
  }

  return (
    <dl className="space-y-1.5 text-xs">
      {present.map((fact) => (
        <div key={fact.label} className="flex gap-4">
          <dt className="text-foreground-alt/60 w-32 shrink-0">{fact.label}</dt>
          <dd
            className={cn(
              'min-w-0 flex-1 break-words',
              fact.mono && 'font-mono tabular-nums',
              fact.tone === 'error'
                ? 'text-destructive'
                : fact.tone === 'warning'
                  ? 'text-warning'
                  : 'text-foreground/90',
            )}
          >
            {typeof fact.value === 'boolean'
              ? fact.value
                ? 'Yes'
                : 'No'
              : String(fact.value)}
          </dd>
        </div>
      ))}
    </dl>
  )
}

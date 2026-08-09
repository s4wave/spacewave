import { useMemo } from 'react'

import { Value } from '@go/github.com/s4wave/spacewave/forge/value/value.pb.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

type ForgeValueLike = Value

interface ForgeValueSetDisplayProps {
  title: string
  values?: ForgeValueLike[]
  emptyLabel: string
}

function valueIdentity(value: ForgeValueLike): string {
  return Array.from(Value.toBinary(value), (byte) =>
    byte.toString(16).padStart(2, '0'),
  ).join('')
}

function describeValue(value: ForgeValueLike): string {
  if (value.worldObjectSnapshot?.key) {
    const rev = value.worldObjectSnapshot.rev
    return rev !== undefined ? `world object @ rev ${rev}` : 'world object'
  }
  if (value.bucketRef?.bucketId) return `bucket ${value.bucketRef.bucketId}`
  if (value.blockRef?.hash) return 'block ref'
  return 'value'
}

export function ForgeValueSetDisplay({
  title,
  values,
  emptyLabel,
}: ForgeValueSetDisplayProps) {
  const rows = useMemo(() => values ?? [], [values])
  const keyedRows = useMemo(() => {
    const occurrences = new Map<string, number>()
    return rows.map((value) => {
      const identity = valueIdentity(value)
      const occurrence = occurrences.get(identity) ?? 0
      occurrences.set(identity, occurrence + 1)
      return { value, key: `${identity}:${occurrence}` }
    })
  }, [rows])

  return (
    <InfoCard title={title}>
      {rows.length === 0 && (
        <div className="text-foreground-alt/40 text-xs">{emptyLabel}</div>
      )}
      {rows.length > 0 && (
        <div className="space-y-2">
          {keyedRows.map(({ value, key }, index) => (
            <div
              key={key}
              className="border-foreground/6 bg-background-card/20 flex items-center justify-between rounded border px-3 py-2"
            >
              <div className="text-foreground text-xs font-medium">
                {value.name || `value-${index + 1}`}
              </div>
              <div className="text-foreground-alt/50 text-xs">
                {describeValue(value)}
              </div>
            </div>
          ))}
        </div>
      )}
    </InfoCard>
  )
}

import { useMemo } from 'react'

import type { ValueSet } from '@go/github.com/s4wave/spacewave/forge/target/target.pb.js'
import type { Value } from '@go/github.com/s4wave/spacewave/forge/value/value.pb.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

type ForgeValueLike = Value
type ForgeValueSetLike = Pick<ValueSet, 'inputs' | 'outputs'>

interface ForgeValueSetDisplayProps {
  title: string
  values?: ForgeValueLike[]
  emptyLabel: string
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

  return (
    <InfoCard title={title}>
      {rows.length === 0 && (
        <div className="text-muted-foreground text-xs">{emptyLabel}</div>
      )}
      {rows.length > 0 && (
        <div className="space-y-2">
          {rows.map((value, index) => (
            <div
              key={`${value.name ?? 'value'}-${index}`}
              className="border-foreground/6 bg-background-card/20 flex items-center justify-between rounded border px-3 py-2"
            >
              <div className="text-foreground text-xs font-medium">
                {value.name || `value-${index + 1}`}
              </div>
              <div className="text-muted-foreground text-xs">
                {describeValue(value)}
              </div>
            </div>
          ))}
        </div>
      )}
    </InfoCard>
  )
}

interface ForgeValueSetPanelsProps {
  valueSet?: ForgeValueSetLike
  inputsTitle?: string
  outputsTitle?: string
  emptyInputsLabel?: string
  emptyOutputsLabel?: string
}

export function ForgeValueSetPanels({
  valueSet,
  inputsTitle = 'Inputs',
  outputsTitle = 'Outputs',
  emptyInputsLabel = 'No inputs',
  emptyOutputsLabel = 'No outputs',
}: ForgeValueSetPanelsProps) {
  return (
    <div className="space-y-3">
      <ForgeValueSetDisplay
        title={inputsTitle}
        values={valueSet?.inputs}
        emptyLabel={emptyInputsLabel}
      />
      <ForgeValueSetDisplay
        title={outputsTitle}
        values={valueSet?.outputs}
        emptyLabel={emptyOutputsLabel}
      />
    </div>
  )
}

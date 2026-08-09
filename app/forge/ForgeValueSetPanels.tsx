import type { ValueSet } from '@go/github.com/s4wave/spacewave/forge/target/target.pb.js'

import { ForgeValueSetDisplay } from './ForgeValueSetDisplay.js'

type ForgeValueSetLike = Pick<ValueSet, 'inputs' | 'outputs'>

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

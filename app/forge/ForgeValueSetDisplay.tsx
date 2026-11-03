import { useMemo } from 'react'

import type { ValueSet } from '@go/github.com/s4wave/spacewave/forge/target/target.pb.js'
import type { Value } from '@go/github.com/s4wave/spacewave/forge/value/value.pb.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
  layout as themeLayout,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

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
  sx,
  title,
  values,
  emptyLabel,
}: ForgeValueSetDisplayProps & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const rows = useMemo(() => values ?? [], [values])

  return (
    <InfoCard sx={sx} title={title}>
      {rows.length === 0 && <div sx={stylexStyles.rule1}>{emptyLabel}</div>}
      {rows.length > 0 && (
        <div sx={false}>
          {rows.map((value, index) => (
            <div
              key={value.name || JSON.stringify(value)}
              sx={[nativeChildSpacing.s2, stylexStyles.rule2]}
            >
              <div sx={stylexStyles.rule3}>
                {value.name || `value-${index + 1}`}
              </div>
              <div sx={stylexStyles.rule4}>{describeValue(value)}</div>
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
  sx,
  valueSet,
  inputsTitle = 'Inputs',
  outputsTitle = 'Outputs',
  emptyInputsLabel = 'No inputs',
  emptyOutputsLabel = 'No outputs',
}: ForgeValueSetPanelsProps & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[false, sx]}>
      <ForgeValueSetDisplay
        sx={nativeChildSpacingNext.s3}
        title={inputsTitle}
        values={valueSet?.inputs}
        emptyLabel={emptyInputsLabel}
      />
      <ForgeValueSetDisplay
        sx={nativeChildSpacingNext.s3}
        title={outputsTitle}
        values={valueSet?.outputs}
        emptyLabel={emptyOutputsLabel}
      />
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})

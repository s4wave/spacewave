import { LuCheck } from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { paddingInline: `calc(${themeLayout['--spacing']} * 2)` },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
    color: themeColors['--color-background'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: { color: themeColors['--color-foreground'] },
  rule6: { color: themeColors['--color-foreground-alt'] },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

export interface PhaseChecklistItem {
  label: string
  done: boolean
  active?: boolean
}

export interface PhaseChecklistProps {
  phases: PhaseChecklistItem[]
  sx?: stylex.StyleXStyles
  className?: never
}

// PhaseChecklist renders an ordered list of phases as a vertical checklist
// with a check, spinner, or pending dot per item.
export function PhaseChecklist({ phases, sx }: PhaseChecklistProps) {
  return (
    <div sx={[stylexStyles3.rule1, sx]}>
      {phases.map((phase) => (
        <PhaseChecklistRow
          key={phase.label}
          {...phase}
          sx={phaseChildren.row}
        />
      ))}
    </div>
  )
}

function PhaseChecklistRow({
  sx,
  label,
  done,
  active,
}: PhaseChecklistItem & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div
      sx={[stylexStyles2.rule1, sx]}
      aria-current={active ? 'step' : undefined}
    >
      {done ? (
        <div sx={stylexStyles2.rule2}>
          <LuCheck sx={stylexStyles.rule1} />
        </div>
      ) : active ? (
        <Spinner variant="brand" />
      ) : (
        <div sx={stylexStyles2.rule3} />
      )}
      <span
        sx={[
          stylexStyles2.rule4,
          done ? stylexStyles2.rule5 : stylexStyles2.rule6,
        ]}
      >
        {label}
      </span>
    </div>
  )
}

// phaseChildren preserves the phase list margins at each rendered row.
const phaseChildren = stylex.create({
  row: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 2 * var(--tw-space-y-reverse))',
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))',
    },
  },
})

import { forgeControlStyles } from '@s4wave/web/style/forge-controls.stylex.js'
import { useCallback } from 'react'

import type { ProcessBindingInfo } from '@s4wave/sdk/space/space.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
  layout as themeLayout,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule6: {
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule7: {
    backgroundColor: {
      default: themeColors['--color-green-600'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-green-700'],
      },
    },
    color: themeColors['--color-white'],
  },
  rule8: {
    backgroundColor: {
      default: themeColors['--color-neutral-600'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-neutral-700'],
      },
    },
    color: themeColors['--color-white'],
  },
  rule9: { cursor: 'not-allowed', opacity: 0.5 },
})

interface ProcessBindingListProps {
  // bindings is the list of process binding info objects.
  bindings: ProcessBindingInfo[]
  // onToggle is called when the user toggles the approval state of a binding.
  // If undefined, the toggle buttons are disabled (not yet connected).
  onToggle?: (objectKey: string, approved: boolean) => void
}

// ProcessBindingList renders a list of process bindings with approve/unapprove toggles.
export function ProcessBindingList({
  sx,
  bindings,
  onToggle,
}: ProcessBindingListProps & { sx?: stylex.StyleXStyles; className?: never }) {
  if (bindings.length === 0) {
    return (
      <div sx={[stylexStyles.rule1, sx]}>No process bindings configured.</div>
    )
  }

  return (
    <div sx={[false, sx]}>
      {bindings.map((binding) => (
        <ProcessBindingRow
          sx={nativeChildSpacing.s1}
          key={binding.objectKey ?? ''}
          binding={binding}
          onToggle={onToggle}
        />
      ))}
    </div>
  )
}

interface ProcessBindingRowProps {
  binding: ProcessBindingInfo
  onToggle?: (objectKey: string, approved: boolean) => void
}

// ProcessBindingRow renders a single process binding entry.
function ProcessBindingRow({
  sx,
  binding,
  onToggle,
}: ProcessBindingRowProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const objectKey = binding.objectKey ?? ''
  const approved = binding.approved ?? false

  const handleToggle = useCallback(() => {
    onToggle?.(objectKey, !approved)
  }, [onToggle, objectKey, approved])

  return (
    <div sx={[stylexStyles.rule2, sx]}>
      <div sx={stylexStyles.rule3}>
        <span sx={stylexStyles.rule4}>{objectKey}</span>
        {binding.typeId && (
          <span sx={stylexStyles.rule5}>{binding.typeId}</span>
        )}
        {binding.decidedAt && (
          <span sx={stylexStyles.rule5}>
            {binding.decidedAt.toLocaleString()}
          </span>
        )}
      </div>
      <button
        type="button"
        disabled={!onToggle}
        onClick={handleToggle}
        sx={[
          [
            stylexStyles.rule6,
            approved ? stylexStyles.rule7 : stylexStyles.rule8,
            !onToggle && stylexStyles.rule9,
          ],
          forgeControlStyles.required,
        ]}
      >
        {approved ? 'Approved' : 'Unapproved'}
      </button>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

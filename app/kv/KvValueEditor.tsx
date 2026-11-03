import { useCallback, useId } from 'react'

import { KvDisplayModeToggle } from './KvDisplayModeToggle.js'
import type { KvDisplayMode } from './kv-encoding.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
  layout as themeLayout,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule3: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 60%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule4: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
})

interface KvValueEditorProps {
  label: string
  draft: string
  onDraftChange: (draft: string) => void
  mode: KvDisplayMode
  onModeChange: (mode: KvDisplayMode) => void
  parseError: string | null
  disabled?: boolean
  rows?: number
  ariaLabel: string
}

// KvValueEditor renders a labeled value textarea with a display-mode toggle and
// inline parse error, shared by the create and update flows.
export function KvValueEditor({
  sx,
  label,
  draft,
  onDraftChange,
  mode,
  onModeChange,
  parseError,
  disabled,
  rows = 6,
  ariaLabel,
}: KvValueEditorProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const textareaId = useId()
  const updateDraft = useCallback(
    (event: React.ChangeEvent<HTMLTextAreaElement>) => {
      onDraftChange(event.target.value)
    },
    [onDraftChange],
  )

  return (
    <div sx={[false, sx]}>
      <div sx={[nativeChildSpacing.s1_5, stylexStyles.rule1]}>
        <label htmlFor={textareaId} sx={stylexStyles.rule2}>
          {label}
        </label>
        <KvDisplayModeToggle mode={mode} onModeChange={onModeChange} />
      </div>
      <textarea
        id={textareaId}
        value={draft}
        onChange={updateDraft}
        disabled={disabled}
        rows={rows}
        aria-label={ariaLabel}
        aria-invalid={parseError ? true : undefined}
        spellCheck={false}
        sx={[
          nativeChildSpacing.s1_5,
          parseError ? stylexStyles.rule3 : stylexStyles.rule4,
        ]}
      />
      {parseError ? (
        <p sx={[nativeChildSpacing.s1_5, stylexStyles.rule5]}>{parseError}</p>
      ) : null}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

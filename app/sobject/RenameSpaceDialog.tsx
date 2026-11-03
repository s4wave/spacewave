import { useCallback, useId, useReducer } from 'react'
import { LuX } from '@s4wave/web/images/icons.js'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import * as stylex from '@stylexjs/stylex'

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  colors as themeColors,
  motion as themeMotion,
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-brand'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'flex',
    justifyContent: 'center',
    outlineStyle: { default: null, ':focus-visible': 'none' },
    position: 'absolute',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    right: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    top: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 4)`,
    },
  },
  rule2: {
    margin: '-1px',
    padding: 0,
    borderWidth: 0,
    overflow: 'hidden',
    clipPath: 'inset(50%)',
    position: 'absolute',
    whiteSpace: 'nowrap',
    height: '1px',
    width: '1px',
  },
  rule3: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule4: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': 0,
    },
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule5: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    borderRadius: themeRadii['--radius-md'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': 0,
    },
  },
  rule7: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-brand'],
    cursor: { default: null, ':disabled': 'not-allowed' },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': 0,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const dialogStyles = stylex.create({
  description: { overflowWrap: 'break-word' },
})

/** RenameSpaceDialogProps supplies the current name and rename operation. */
export interface RenameSpaceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  spaceName: string
  onConfirm: (newName: string) => Promise<void>
}

interface RenameState {
  submitting: boolean
  error?: string
  source: string
  value: string
}

type RenameAction =
  | { type: 'reset'; value: string }
  | { type: 'set-value'; source: string; value: string }
  | { type: 'submit' }
  | { type: 'fail'; error: string }

function renameReducer(state: RenameState, action: RenameAction): RenameState {
  switch (action.type) {
    case 'reset':
      return { submitting: false, source: action.value, value: action.value }
    case 'set-value':
      return { ...state, source: action.source, value: action.value }
    case 'submit':
      return { ...state, submitting: true, error: undefined }
    case 'fail':
      return { ...state, submitting: false, error: action.error }
  }
}

/** RenameSpaceDialog prompts for a Space display name and submits the change. */
export function RenameSpaceDialog({
  open,
  onOpenChange,
  spaceName,
  onConfirm,
}: RenameSpaceDialogProps) {
  // Keep the draft tied to the current Space name.
  const [state, dispatch] = useReducer(renameReducer, {
    submitting: false,
    source: spaceName,
    value: spaceName,
  })
  const spaceNameId = useId()
  const value = state.source === spaceName ? state.value : spaceName

  // Discard the draft when the dialog closes.
  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        dispatch({ type: 'reset', value: spaceName })
      }
      onOpenChange(next)
    },
    [onOpenChange, spaceName],
  )

  // Allow submission only for a changed, nonempty name.
  const trimmed = value.trim()
  const canSubmit =
    trimmed.length > 0 && trimmed !== spaceName && !state.submitting
  const handleInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  // Submit the current name and keep a failed draft available.
  const handleSubmit = useCallback(async () => {
    if (!canSubmit) return
    dispatch({ type: 'submit' })
    try {
      await onConfirm(trimmed)
      handleOpenChange(false)
    } catch (err) {
      dispatch({
        type: 'fail',
        error: err instanceof Error ? err.message : 'Rename failed',
      })
    }
  }, [canSubmit, onConfirm, trimmed, handleOpenChange])

  // Style the editable name consistently across pointer types.

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent showCloseButton={false}>
        <button
          type="button"
          onClick={() => handleOpenChange(false)}
          sx={stylexStyles2.rule1}
        >
          <LuX sx={stylexStyles.rule1} />
          <span sx={stylexStyles2.rule2}>Close</span>
        </button>

        <DialogHeader>
          <DialogTitle>Rename Space</DialogTitle>
          <DialogDescription sx={dialogStyles.description}>
            Enter a new display name for &ldquo;{spaceName}&rdquo;.
          </DialogDescription>
        </DialogHeader>

        <div>
          <label sx={stylexStyles2.rule3} htmlFor={spaceNameId}>
            Space name
          </label>
          <input
            id={spaceNameId}
            ref={handleInputRef}
            value={value}
            onChange={(e) =>
              dispatch({
                type: 'set-value',
                source: spaceName,
                value: e.target.value,
              })
            }
            placeholder={spaceName}
            sx={[stylexStyles2.rule4]}
            aria-label="New space name"
            onKeyDown={(e) => {
              if (
                e.key === 'Enter' &&
                !e.nativeEvent.isComposing &&
                canSubmit
              ) {
                e.preventDefault()
                void handleSubmit()
              }
            }}
          />
        </div>

        {state.error && <p sx={stylexStyles2.rule5}>{state.error}</p>}

        <DialogFooter>
          <button
            type="button"
            onClick={() => handleOpenChange(false)}
            disabled={state.submitting}
            sx={stylexStyles2.rule6}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void handleSubmit()}
            disabled={!canSubmit}
            sx={[stylexStyles2.rule7]}
          >
            {state.submitting ? 'Saving…' : 'Save'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

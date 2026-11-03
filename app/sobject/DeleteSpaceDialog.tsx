import { useCallback, useId, useState } from 'react'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule2: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
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
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule3: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
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
  },
  rule5: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-destructive'],
    cursor: { default: null, ':disabled': 'not-allowed' },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

export interface DeleteSpaceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  spaceName: string
  onConfirm: () => Promise<void>
}

// DeleteSpaceDialog prompts the user to type the space name to confirm deletion.
export function DeleteSpaceDialog({
  open,
  onOpenChange,
  spaceName,
  onConfirm,
}: DeleteSpaceDialogProps) {
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string>()
  const [confirmText, setConfirmText] = useState('')
  const confirmInputId = useId()

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        setConfirmText('')
        setError(undefined)
        setSubmitting(false)
      }
      onOpenChange(next)
    },
    [onOpenChange],
  )

  const handleDelete = useCallback(async () => {
    setSubmitting(true)
    setError(undefined)
    try {
      await onConfirm()
      handleOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
      setSubmitting(false)
    }
  }, [onConfirm, handleOpenChange])

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Space</DialogTitle>
          <DialogDescription>
            This will permanently delete &ldquo;{spaceName}&rdquo; and all its
            data. Type the space name to confirm.
          </DialogDescription>
        </DialogHeader>

        <div>
          <label htmlFor={confirmInputId} sx={stylexStyles.rule1}>
            Space name
          </label>
          <input
            id={confirmInputId}
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
            placeholder={spaceName}
            sx={[stylexStyles.rule2]}
            onKeyDown={(e) => {
              if (e.nativeEvent.isComposing) return
              if (
                e.key === 'Enter' &&
                confirmText === spaceName &&
                !submitting
              ) {
                void handleDelete()
              }
            }}
          />
        </div>

        {error && <p sx={stylexStyles.rule3}>{error}</p>}

        <DialogFooter>
          <button
            type="button"
            onClick={() => handleOpenChange(false)}
            disabled={submitting}
            sx={stylexStyles.rule4}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void handleDelete()}
            disabled={confirmText !== spaceName || submitting}
            sx={[stylexStyles.rule5]}
          >
            {submitting ? 'Deleting…' : 'Delete Space'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

import { useCallback, useState } from 'react'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import { TypedConfirmField } from './TypedConfirmField.js'

import * as stylex from '@stylexjs/stylex'
import {
  radii as themeRadii,
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-destructive'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-destructive'],
  },
  rule4: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-destructive'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule7: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-destructive'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-destructive'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

export interface DeleteAccountDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isCloud: boolean
  onConfirm: () => Promise<void>
}

type Step = 'warning' | 'confirm' | 'final'

// DeleteAccountDialog is a multi-step confirmation dialog for account deletion.
export function DeleteAccountDialog({
  open,
  onOpenChange,
  isCloud,
  onConfirm,
}: DeleteAccountDialogProps) {
  const [step, setStep] = useState<Step>('warning')
  const [typedName, setTypedName] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string>()

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        setStep('warning')
        setTypedName('')
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
          <DialogTitle>
            {isCloud ? 'Delete Account' : 'Delete Local Data'}
          </DialogTitle>
          {step === 'warning' && (
            <DialogDescription>
              {isCloud
                ? 'Deleting your cloud account requires extra verification. You will confirm by email code or from account.spacewave.app, billing will finalize immediately, and a 24-hour undo window will begin.'
                : 'This will permanently delete your local account and all its data.'}
            </DialogDescription>
          )}
        </DialogHeader>

        {step === 'warning' && (
          <DialogFooter>
            <button
              type="button"
              onClick={() => handleOpenChange(false)}
              sx={stylexStyles.rule1}
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => setStep('confirm')}
              sx={[stylexStyles.rule2]}
            >
              Review deletion
            </button>
          </DialogFooter>
        )}

        {step === 'confirm' && (
          <>
            <TypedConfirmField
              label={
                <>
                  Type <span sx={stylexStyles.rule3}>DELETE</span> to confirm
                </>
              }
              value={typedName}
              onChange={setTypedName}
              placeholder="DELETE"
              canSubmit={typedName === 'DELETE'}
              onSubmit={() => setStep('final')}
            />
            <DialogFooter>
              <button
                type="button"
                onClick={() => {
                  setStep('warning')
                  setTypedName('')
                }}
                sx={stylexStyles.rule1}
              >
                Back
              </button>
              <button
                type="button"
                disabled={typedName !== 'DELETE'}
                onClick={() => setStep('final')}
                sx={[stylexStyles.rule4]}
              >
                Confirm
              </button>
            </DialogFooter>
          </>
        )}

        {step === 'final' && (
          <>
            <p sx={stylexStyles.rule5}>
              {isCloud
                ? 'Continue to the delete confirmation screen to send the email code and finalize the 24-hour delete countdown.'
                : 'This action cannot be undone. All local data will be permanently deleted.'}
            </p>

            {error && <p sx={stylexStyles.rule6}>{error}</p>}

            <DialogFooter>
              <button
                type="button"
                onClick={() => setStep('confirm')}
                disabled={submitting}
                sx={stylexStyles.rule1}
              >
                Back
              </button>
              <button
                type="button"
                onClick={() => void handleDelete()}
                disabled={submitting}
                sx={[stylexStyles.rule7]}
              >
                {submitting
                  ? 'Deleting…'
                  : isCloud
                    ? 'Continue to Delete Flow'
                    : 'Delete Everything'}
              </button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

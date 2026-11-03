import { LuUnlink } from '@s4wave/web/images/icons.js'

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
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule2: {
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-destructive'],
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// DetachAssignmentTarget identifies the principal whose billing pointer is
// being cleared.
export interface DetachAssignmentTarget {
  ownerType: 'account' | 'organization'
  ownerId: string
  label: string
}

export interface DetachAssignmentDialogProps {
  target: DetachAssignmentTarget | null
  busy: boolean
  onCancel: () => void
  onConfirm: () => void
}

// DetachAssignmentDialog is the shared confirmation modal for clearing a
// billing account assignment from a principal. Controlled by the caller.
export function DetachAssignmentDialog({
  target,
  busy,
  onCancel,
  onConfirm,
}: DetachAssignmentDialogProps) {
  return (
    <Dialog
      open={target !== null}
      onOpenChange={(open) => {
        if (!open) onCancel()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Detach billing from {target?.label}?</DialogTitle>
          <DialogDescription>
            The billing account assigned to{' '}
            <span sx={stylexStyles2.rule1}>{target?.label}</span> will be
            cleared. Resources owned by this principal will lose billing
            coverage and may move to the free tier or be blocked until a billing
            account is re-assigned.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <button
            type="button"
            onClick={onCancel}
            disabled={busy}
            sx={stylexStyles2.rule2}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={busy}
            sx={stylexStyles2.rule3}
          >
            <LuUnlink sx={stylexStyles.rule1} />
            <span>{busy ? 'Detaching…' : 'Detach'}</span>
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

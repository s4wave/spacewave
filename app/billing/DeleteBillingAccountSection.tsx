import { useCallback, useId, useState } from 'react'
import { LuTrash2 } from '@s4wave/web/images/icons.js'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'
import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { deleteBillingAccountDisabledReason } from './billing-utils.js'

import * as stylex from '@stylexjs/stylex'

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    textTransform: 'uppercase',
  },
  rule2: { display: 'inline-flex' },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-destructive'],
  },
  rule7: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule9: {
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
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule10: {
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

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { maxWidth: 'var(--container-xs)' },
})

export interface DeleteBillingAccountSectionProps {
  billingAccountId: string
  displayName: string
  status?: BillingStatus
  assigneeCount: number
  disabledReasonOverride?: string | null
  onDeleted: () => void
}

// DeleteBillingAccountSection renders the billing-account delete action and its
// confirmation dialog.
export function DeleteBillingAccountSection({
  sx,
  billingAccountId,
  displayName,
  status,
  assigneeCount,
  disabledReasonOverride,
  onDeleted,
}: DeleteBillingAccountSectionProps & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  // Keep the confirmation dialog, typed acknowledgement and deletion result together.
  const session = SessionContext.useContext().value
  const [open, setOpen] = useState(false)
  const [confirmText, setConfirmText] = useState('')
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const confirmInputId = useId()

  // Derive deletion eligibility from the account status and explicit acknowledgement.
  const disabledReason =
    disabledReasonOverride ??
    deleteBillingAccountDisabledReason(status, assigneeCount)
  const deleteDisabled = !session || deleting || !!disabledReason
  const confirmed = confirmText.trim() === 'DELETE'
  const handleConfirmInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  // Keep the dialog open during deletion and clear acknowledgement when it closes.
  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (deleting) return
      if (!next) {
        setConfirmText('')
        setError(null)
      }
      setOpen(next)
    },
    [deleting],
  )

  // Delete only the acknowledged account and report the completed operation.
  const handleDelete = useCallback(async () => {
    if (!session || !billingAccountId || deleting || !confirmed) return
    setDeleting(true)
    setError(null)
    try {
      await session.spacewave.deleteBillingAccount(billingAccountId)
      handleOpenChange(false)
      onDeleted()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
      setDeleting(false)
    }
  }, [
    session,
    billingAccountId,
    deleting,
    confirmed,
    handleOpenChange,
    onDeleted,
  ])

  // Expose the deletion trigger with the current eligibility reason.
  const button = (
    <DashboardButton
      sx={!disabledReason && deleteChildren.child}
      icon={<LuTrash2 sx={stylexStyles2.rule1} />}
      onClick={() => handleOpenChange(true)}
      disabled={deleteDisabled}
      variant="destructive"
    >
      Delete billing account
    </DashboardButton>
  )

  return (
    <div sx={sx}>
      <div sx={[stylexStyles3.rule1, deleteChildren.child]}>Danger Zone</div>
      {disabledReason ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <span sx={[stylexStyles3.rule2, deleteChildren.child]}>
              {button}
            </span>
          </TooltipTrigger>
          <TooltipContent side="top" sx={stylexStyles.rule1}>
            {disabledReason}
          </TooltipContent>
        </Tooltip>
      ) : (
        button
      )}
      <div sx={[stylexStyles3.rule3, deleteChildren.child]}>
        Permanently removes this billing account record after cancellation and
        detach are complete.
      </div>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Billing Account</DialogTitle>
            <DialogDescription>
              This permanently deletes{' '}
              <span sx={stylexStyles3.rule4}>
                {displayName || 'this billing account'}
              </span>
              . It cannot be assigned or reused again.
            </DialogDescription>
          </DialogHeader>
          <div>
            <label htmlFor={confirmInputId} sx={stylexStyles3.rule5}>
              Type <span sx={stylexStyles3.rule6}>DELETE</span> to confirm.
            </label>
            <input
              id={confirmInputId}
              ref={handleConfirmInputRef}
              type="text"
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder="DELETE"
              sx={[stylexStyles3.rule7]}
              onKeyDown={(e) => {
                if (e.nativeEvent.isComposing) return
                if (e.key === 'Enter' && confirmed && !deleting) {
                  void handleDelete()
                }
              }}
            />
          </div>
          {error && <div sx={stylexStyles3.rule8}>{error}</div>}
          <DialogFooter>
            <button
              type="button"
              onClick={() => handleOpenChange(false)}
              disabled={deleting}
              sx={stylexStyles3.rule9}
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => void handleDelete()}
              disabled={!confirmed || deleting}
              sx={[stylexStyles3.rule10]}
            >
              {deleting ? 'Deleting…' : 'Delete billing account'}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

// deleteChildren preserves spacing on the actual trigger or tooltip span.
const deleteChildren = stylex.create({
  child: {
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

import * as ownerStylex from '@stylexjs/stylex'

import { useCallback, useId, useState } from 'react'
import { LuTrash2 } from '@s4wave/web/images/icons.js'

import {
  SessionContext,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
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
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

import { groupMarker as groupMarker4 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = ownerStylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      },
    },
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive-foreground'],
      },
    },
    cursor: 'not-allowed',
    display: 'flex',
    opacity: 0.5,
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive-foreground'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
})

const stylexStyles2 = ownerStylex.create({
  rule1: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule3: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          `color-mix(in oklab, ${themeColors['--color-destructive-foreground']} 20%, transparent)`,
      },
    },
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule4: { flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: {
      default: themeColors['--color-destructive'],
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-destructive-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
  },
  rule6: {
    WebkitUserSelect: 'none',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 80%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          `color-mix(in oklab, ${themeColors['--color-destructive-foreground']} 80%, transparent)`,
      },
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
  },
  rule7: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule8: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule9: {
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
  rule10: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule11: {
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
    cursor: { default: null, ':disabled': 'not-allowed' },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule12: {
    borderColor: themeColors['--color-destructive'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
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

const stylexStyles = stylex.create({
  rule1: {
    color: {
      default: themeColors['--color-destructive'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-destructive-foreground'],
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export interface OrgActionsSectionProps {
  orgId: string
  displayName: string
  spaceCount: number
}

// OrgActionsSection renders owner-only actions (currently Delete) using the
// same button-card layout as SessionDetails Actions.
export function OrgActionsSection({
  sx,
  orgId,
  displayName,
  spaceCount,
}: OrgActionsSectionProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const [dialogOpen, setDialogOpen] = useState(false)
  const blocked = spaceCount > 0

  return (
    <section sx={sx}>
      <div sx={stylexStyles2.rule1}>
        <h2 sx={stylexStyles2.rule2}>Actions</h2>
      </div>

      <div>
        <button
          type="button"
          onClick={() => setDialogOpen(true)}
          disabled={blocked}
          sx={[
            nativeChildSpacing.s2,
            blocked
              ? [stylexStyles3.rule1, groupMarker4]
              : [stylexStyles3.rule2, groupMarker4],
          ]}
        >
          <div sx={stylexStyles2.rule3}>
            <LuTrash2 sx={stylexStyles.rule1} />
          </div>
          <div sx={stylexStyles2.rule4}>
            <h4 sx={stylexStyles2.rule5}>Delete Organization</h4>
            <p sx={stylexStyles2.rule6}>
              {blocked
                ? `Remove or transfer all ${spaceCount} space${spaceCount !== 1 ? 's' : ''} before deleting`
                : 'Permanently delete this organization'}
            </p>
          </div>
        </button>
      </div>

      <DeleteOrgDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        orgId={orgId}
        displayName={displayName}
      />
    </section>
  )
}

interface DeleteOrgDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgId: string
  displayName: string
}

// DeleteOrgDialog confirms deletion by asking the owner to type the
// organization name.
function DeleteOrgDialog({
  open,
  onOpenChange,
  orgId,
  displayName,
}: DeleteOrgDialogProps) {
  const session = SessionContext.useContext().value
  const navigateSession = useSessionNavigate()
  const [confirmText, setConfirmText] = useState('')
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const confirmInputId = useId()

  const confirmed =
    confirmText.trim().toLowerCase() === displayName.trim().toLowerCase()
  const handleConfirmInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (deleting) return
      if (!next) {
        setConfirmText('')
        setError(null)
      }
      onOpenChange(next)
    },
    [deleting, onOpenChange],
  )

  const handleDelete = useCallback(async () => {
    if (!session || !confirmed || deleting) return
    setDeleting(true)
    setError(null)
    try {
      await session.spacewave.deleteOrganization(orgId)
      navigateSession({ path: '' })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete')
      setDeleting(false)
    }
  }, [session, orgId, confirmed, deleting, navigateSession])

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Organization</DialogTitle>
          <DialogDescription>
            This will permanently delete the organization and cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <div>
          <label htmlFor={confirmInputId} sx={stylexStyles2.rule7}>
            Type <span sx={stylexStyles2.rule8}>{displayName}</span> to confirm.
          </label>
          <input
            id={confirmInputId}
            ref={handleConfirmInputRef}
            type="text"
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
            placeholder={displayName}
            sx={[stylexStyles2.rule9]}
            onKeyDown={(e) => {
              if (e.nativeEvent.isComposing) return
              if (e.key === 'Enter' && confirmed && !deleting) {
                void handleDelete()
              }
            }}
          />
        </div>
        {error && <p sx={stylexStyles2.rule10}>{error}</p>}
        <DialogFooter>
          <button
            type="button"
            onClick={() => handleOpenChange(false)}
            disabled={deleting}
            sx={stylexStyles2.rule11}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void handleDelete()}
            disabled={!confirmed || deleting}
            sx={[stylexStyles2.rule12]}
          >
            {deleting ? 'Deleting…' : 'Delete Organization'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
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

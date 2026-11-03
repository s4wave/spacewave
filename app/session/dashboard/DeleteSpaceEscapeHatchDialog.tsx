import { useCallback, useMemo, useState } from 'react'
import {
  LuBoxes,
  LuCircleAlert,
  LuCircleCheck,
  LuShieldAlert,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'
import { useWatchStateRpc } from '@aptre/bldr-react'

import type { Session } from '@s4wave/sdk/session/session.js'
import {
  WatchResourcesListRequest,
  WatchResourcesListResponse,
  WatchSharedObjectHealthRequest,
  WatchSharedObjectHealthResponse,
} from '@s4wave/sdk/session/session.pb.js'
import {
  SharedObjectHealthStatus,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'
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
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule3: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    accentColor: themeColors['--color-destructive'],
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    wordBreak: 'break-all',
    color: themeColors['--color-destructive'],
  },
  rule6: {
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
  rule7: {
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
  rule8: {
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
  rule9: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule10: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule11: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
  },
  rule12: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      },
    },
  },
  rule13: { minWidth: 0, flex: 1 },
  rule14: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule15: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '.65rem',
    lineHeight: 1.5,
    wordBreak: 'break-all',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    WebkitUserSelect: 'text',
    userSelect: 'text',
  },
  rule16: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule17: {
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule18: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule19: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    wordBreak: 'break-all',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    WebkitUserSelect: 'text',
    userSelect: 'text',
  },
  rule20: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule21: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule22: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-destructive'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule3: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule4: { color: themeColors['--color-destructive'] },
  rule5: { color: themeColors['--color-foreground-alt'] },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule7: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

export interface DeleteSpaceEscapeHatchDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  session: Session | null
}

type Step = 'select' | 'warning' | 'final'

interface SpaceChoice {
  id: string
  name: string
  // hasName is true when the space advertises a user-visible display name.
  hasName: boolean
}

// useSpaceChoices watches the session's spaces while the dialog is open.
// loading is true until the first list arrives.
function useSpaceChoices(open: boolean, session: Session | null) {
  const resourcesList = useWatchStateRpc(
    useCallback(
      (req: WatchResourcesListRequest, signal: AbortSignal) =>
        open && session ? session.watchResourcesList(req, signal) : null,
      [open, session],
    ),
    {},
    WatchResourcesListRequest.equals,
    WatchResourcesListResponse.equals,
  )

  const choices = useMemo<SpaceChoice[]>(() => {
    const entries = resourcesList?.spacesList ?? []
    return entries.flatMap((entry): SpaceChoice[] => {
      const id = entry.entry?.ref?.providerResourceRef?.id ?? ''
      if (!id) return []
      const name = entry.spaceMeta?.name?.trim() ?? ''
      return [
        {
          id,
          name: name || id,
          hasName: name.length > 0,
        },
      ]
    })
  }, [resourcesList])

  return { choices, loading: !resourcesList }
}

// useSelectedSpaceHealth watches the health of the chosen space. It only
// watches once the user has picked a space, which avoids fanning out one Watch
// stream per space in the list.
function useSelectedSpaceHealth(
  open: boolean,
  session: Session | null,
  selectedId: string,
): SharedObjectHealth | null {
  const healthResp = useWatchStateRpc(
    useCallback(
      (req: WatchSharedObjectHealthRequest, signal: AbortSignal) =>
        open && session && selectedId
          ? session.watchSharedObjectHealth(req, signal)
          : null,
      [open, session, selectedId],
    ),
    selectedId ? { sharedObjectId: selectedId } : null,
    WatchSharedObjectHealthRequest.equals,
    WatchSharedObjectHealthResponse.equals,
  )
  return healthResp?.health ?? null
}

// useSpaceDeletion deletes the selected space through the session and tracks
// the pending and failed state. onDeleted runs after a successful delete.
function useSpaceDeletion(
  session: Session | null,
  selected: SpaceChoice | null,
  onDeleted: () => void,
) {
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string>()

  const deleteSpace = async () => {
    if (!session || !selected) return
    setSubmitting(true)
    setError(undefined)
    try {
      await session.deleteSpace(selected.id)
      onDeleted()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
      setSubmitting(false)
    }
  }

  return {
    submitting,
    error,
    deleteSpace,
    clearError: () => setError(undefined),
    reset: () => {
      setError(undefined)
      setSubmitting(false)
    },
  }
}

// DeleteSpaceEscapeHatchDialog is a stepwise destructive flow for deleting a
// space without mounting it, used when a space cannot be opened normally.
export function DeleteSpaceEscapeHatchDialog({
  open,
  onOpenChange,
  session,
}: DeleteSpaceEscapeHatchDialogProps) {
  const [step, setStep] = useState<Step>('select')
  const [selectedId, setSelectedId] = useState('')
  const [selectedSnapshot, setSelectedSnapshot] = useState<SpaceChoice | null>(
    null,
  )
  const [acknowledged, setAcknowledged] = useState(false)
  const [typedConfirm, setTypedConfirm] = useState('')

  const { choices, loading } = useSpaceChoices(open, session)
  const liveSelected = choices.find((c) => c.id === selectedId) ?? null
  const selected = selectedSnapshot ?? liveSelected
  const health = useSelectedSpaceHealth(open, session, selectedId)
  const deletion = useSpaceDeletion(session, selected, () =>
    handleOpenChange(false),
  )
  const { submitting, error } = deletion

  // handleOpenChange resets the flow when the dialog closes.
  function handleOpenChange(next: boolean) {
    if (!next) {
      setStep('select')
      setSelectedId('')
      setSelectedSnapshot(null)
      setAcknowledged(false)
      setTypedConfirm('')
      deletion.reset()
    }
    onOpenChange(next)
  }

  function handleSelect(id: string) {
    setSelectedId(id)
    setSelectedSnapshot(choices.find((c) => c.id === id) ?? null)
    deletion.clearError()
  }

  // The typed name must equal the selected space's name; a missing selection
  // never matches.
  const canDelete =
    acknowledged && !submitting && typedConfirm === selected?.name

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle variant="withIcon">
            <LuShieldAlert sx={stylexStyles.rule1} />
            Delete a Space
          </DialogTitle>
          <DeleteSpaceDescription step={step} selected={selected} />
        </DialogHeader>

        {step === 'select' && (
          <SpaceSelectList
            choices={choices}
            loading={loading}
            selectedId={selectedId}
            onSelect={handleSelect}
          />
        )}

        {step === 'warning' && selected && (
          <DeleteWarningStep
            space={selected}
            health={health}
            acknowledged={acknowledged}
            onAcknowledgedChange={setAcknowledged}
          />
        )}

        {step === 'final' && selected && (
          <DeleteFinalStep
            space={selected}
            health={health}
            typedConfirm={typedConfirm}
            onTypedConfirmChange={setTypedConfirm}
            canDelete={canDelete}
            onDelete={() => void deletion.deleteSpace()}
          />
        )}

        {error && <p sx={stylexStyles2.rule1}>{error}</p>}

        <DialogFooter>
          {step === 'select' && (
            <SelectStepActions
              canContinue={!!selected}
              onCancel={() => handleOpenChange(false)}
              onContinue={() => setStep('warning')}
            />
          )}

          {step === 'warning' && (
            <WarningStepActions
              acknowledged={acknowledged}
              onBack={() => {
                setStep('select')
                setAcknowledged(false)
              }}
              onContinue={() => setStep('final')}
            />
          )}

          {step === 'final' && (
            <FinalStepActions
              submitting={submitting}
              canDelete={canDelete}
              onBack={() => {
                setStep('warning')
                setTypedConfirm('')
              }}
              onDelete={() => void deletion.deleteSpace()}
            />
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

interface DeleteSpaceDescriptionProps {
  step: Step
  selected: SpaceChoice | null
}

// DeleteSpaceDescription explains the current step under the dialog title.
function DeleteSpaceDescription({
  step,
  selected,
}: DeleteSpaceDescriptionProps) {
  if (step === 'select') {
    return (
      <DialogDescription>
        Pick a space to permanently delete. Use this only when a space will not
        open and cannot be removed from inside the space itself. Deletion is
        final and does not require the space to mount.
      </DialogDescription>
    )
  }
  if (!selected) return null
  if (step === 'warning') {
    return (
      <DialogDescription>
        This will permanently delete{' '}
        <span sx={stylexStyles2.rule2}>
          {selected.hasName ? selected.name : selected.id}
        </span>{' '}
        and all of its data. Confirm you understand before continuing.
      </DialogDescription>
    )
  }
  return (
    <DialogDescription>
      Type the {selected.hasName ? 'space name' : 'shared object id'} exactly to
      confirm.
    </DialogDescription>
  )
}

interface DeleteWarningStepProps {
  space: SpaceChoice
  health: SharedObjectHealth | null
  acknowledged: boolean
  onAcknowledgedChange: (acknowledged: boolean) => void
}

// DeleteWarningStep asks the user to acknowledge that deletion is permanent.
function DeleteWarningStep({
  space,
  health,
  acknowledged,
  onAcknowledgedChange,
}: DeleteWarningStepProps) {
  return (
    <div>
      <SelectedSpaceSummary
        sx={nativeChildSpacing.s3}
        space={space}
        health={health}
      />
      <label sx={[nativeChildSpacing.s3, stylexStyles2.rule3]}>
        <input
          type="checkbox"
          checked={acknowledged}
          onChange={(e) => onAcknowledgedChange(e.target.checked)}
          sx={stylexStyles2.rule4}
          aria-label="Confirm delete is permanent"
        />
        <span>
          I understand this permanently deletes the space and its data.
        </span>
      </label>
    </div>
  )
}

interface DeleteFinalStepProps {
  space: SpaceChoice
  health: SharedObjectHealth | null
  typedConfirm: string
  onTypedConfirmChange: (value: string) => void
  canDelete: boolean
  onDelete: () => void
}

// DeleteFinalStep asks the user to type the space name to confirm deletion.
function DeleteFinalStep({
  space,
  health,
  typedConfirm,
  onTypedConfirmChange,
  canDelete,
  onDelete,
}: DeleteFinalStepProps) {
  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
      }
    >
      <SelectedSpaceSummary space={space} health={health} />
      <TypedConfirmField
        label={
          <>
            Type <span sx={stylexStyles2.rule5}>{space.name}</span> to confirm
          </>
        }
        value={typedConfirm}
        onChange={onTypedConfirmChange}
        placeholder={space.name}
        ariaLabel="Confirm space name or id"
        canSubmit={canDelete}
        onSubmit={onDelete}
      />
    </div>
  )
}

interface SelectStepActionsProps {
  canContinue: boolean
  onCancel: () => void
  onContinue: () => void
}

// SelectStepActions renders the footer buttons of the select step.
function SelectStepActions({
  canContinue,
  onCancel,
  onContinue,
}: SelectStepActionsProps) {
  return (
    <>
      <button type="button" onClick={onCancel} sx={stylexStyles2.rule6}>
        Cancel
      </button>
      <button
        type="button"
        disabled={!canContinue}
        onClick={onContinue}
        sx={[stylexStyles2.rule7]}
      >
        Review space deletion
      </button>
    </>
  )
}

interface WarningStepActionsProps {
  acknowledged: boolean
  onBack: () => void
  onContinue: () => void
}

// WarningStepActions renders the footer buttons of the warning step.
function WarningStepActions({
  acknowledged,
  onBack,
  onContinue,
}: WarningStepActionsProps) {
  return (
    <>
      <button type="button" onClick={onBack} sx={stylexStyles2.rule6}>
        Back
      </button>
      <button
        type="button"
        disabled={!acknowledged}
        onClick={onContinue}
        sx={[stylexStyles2.rule7]}
      >
        Confirm space deletion
      </button>
    </>
  )
}

interface FinalStepActionsProps {
  submitting: boolean
  canDelete: boolean
  onBack: () => void
  onDelete: () => void
}

// FinalStepActions renders the footer buttons of the final step.
function FinalStepActions({
  submitting,
  canDelete,
  onBack,
  onDelete,
}: FinalStepActionsProps) {
  return (
    <>
      <button
        type="button"
        onClick={onBack}
        disabled={submitting}
        sx={stylexStyles2.rule6}
      >
        Back
      </button>
      <button
        type="button"
        onClick={onDelete}
        disabled={!canDelete}
        sx={[stylexStyles2.rule8]}
      >
        {submitting ? 'Deleting…' : 'Delete Space'}
      </button>
    </>
  )
}

interface SpaceSelectListProps {
  choices: SpaceChoice[]
  loading: boolean
  selectedId: string
  onSelect: (id: string) => void
}

// SpaceSelectList renders the chooser used in the first dialog step.
function SpaceSelectList({
  choices,
  loading,
  selectedId,
  onSelect,
}: SpaceSelectListProps) {
  if (loading) {
    return (
      <div sx={stylexStyles2.rule9}>
        <LuBoxes sx={stylexStyles.rule2} />
        Loading spaces…
      </div>
    )
  }
  if (choices.length === 0) {
    return (
      <div sx={stylexStyles2.rule9}>
        <LuBoxes sx={stylexStyles.rule2} />
        No spaces in this session.
      </div>
    )
  }
  return (
    <div
      role="radiogroup"
      aria-label="Spaces"
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-1.5 */ 'max-h-72 space-y-1.5 overflow-y-auto pr-1'
      }
    >
      {choices.map((choice) => {
        const isSelected = choice.id === selectedId
        return (
          <button
            key={choice.id}
            type="button"
            role="radio"
            aria-checked={isSelected}
            onClick={() => onSelect(choice.id)}
            sx={[
              stylexStyles2.rule10,
              isSelected ? stylexStyles2.rule11 : stylexStyles2.rule12,
            ]}
          >
            <LuBoxes
              sx={[
                stylexStyles.rule3,
                isSelected ? stylexStyles.rule4 : stylexStyles.rule5,
              ]}
            />
            <div sx={stylexStyles2.rule13}>
              <div sx={stylexStyles2.rule14}>
                {choice.hasName ? choice.name : 'Unnamed space'}
              </div>
              <div sx={stylexStyles2.rule15}>{choice.id}</div>
            </div>
          </button>
        )
      })}
    </div>
  )
}

interface SelectedSpaceSummaryProps {
  space: SpaceChoice
  health: SharedObjectHealth | null
}

// SelectedSpaceSummary shows identity and health for the chosen space.
function SelectedSpaceSummary({
  sx,
  space,
  health,
}: SelectedSpaceSummaryProps & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  return (
    <div sx={[stylexStyles2.rule16, sx]}>
      <div sx={stylexStyles2.rule17}>Space</div>
      <div sx={stylexStyles2.rule18}>
        {space.hasName ? space.name : 'Unnamed space'}
      </div>
      <div sx={stylexStyles2.rule19}>{space.id}</div>
      <HealthBadge health={health} />
    </div>
  )
}

interface HealthBadgeProps {
  health: SharedObjectHealth | null
}

// HealthBadge surfaces broken/degraded status inline on the confirmation step.
function HealthBadge({ health }: HealthBadgeProps) {
  if (!health) return null
  const status = health.status ?? SharedObjectHealthStatus.UNKNOWN
  if (status === SharedObjectHealthStatus.READY) {
    return (
      <div sx={stylexStyles2.rule20}>
        <LuCircleCheck sx={stylexStyles.rule6} />
        Space is reachable.
      </div>
    )
  }
  if (status === SharedObjectHealthStatus.LOADING) {
    return (
      <div sx={stylexStyles2.rule20}>
        <LuCircleAlert sx={stylexStyles.rule6} />
        Checking status…
      </div>
    )
  }
  if (status === SharedObjectHealthStatus.DEGRADED) {
    return (
      <div sx={stylexStyles2.rule21}>
        <LuTriangleAlert sx={stylexStyles.rule7} />
        Degraded. Some data may be partially available.
      </div>
    )
  }
  if (status === SharedObjectHealthStatus.CLOSED) {
    return (
      <div sx={stylexStyles2.rule22}>
        <LuShieldAlert sx={stylexStyles.rule7} />
        Cannot mount. This space is broken and must be deleted from here.
      </div>
    )
  }
  return null
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

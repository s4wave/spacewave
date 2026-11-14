import { useCallback, type FocusEvent, type ReactNode } from 'react'
import { LuArrowLeft, LuCheck, LuTrash2 } from 'react-icons/lu'

import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'
import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'
import { Button } from '@s4wave/web/ui/button.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { cn } from '@s4wave/web/style/utils.js'

import { WizardField } from './WizardField.js'
import { WizardGone } from './WizardGone.js'
import type { WizardFinishState } from './wizard-finish.js'
import type { WizardCancelState } from './useWizardState.js'

export interface WizardShellProps {
  // icon marks the kind of object the wizard creates.
  icon?: ReactNode
  title: string
  step: number
  totalSteps?: number
  stepName?: string
  localName: string
  onUpdateName: (name: string) => void
  onBack: () => void
  canBack?: boolean
  // Delete wizard in the bar runs onCancel; cancel is its state, busy while
  // the delete runs and with the reason beneath the bar once it failed.
  onCancel: () => void
  cancel: WizardCancelState
  // Content slot rendered between header and buttons.
  children?: ReactNode
  // Name input configuration.
  nameLabel?: string
  namePlaceholder?: string
  nameHelp?: string
  nameError?: string
  // Which step shows the name input. Defaults to 0.
  nameStep?: number
  // Selects the existing default name when the name input receives focus.
  selectNameOnFocus?: boolean
  // Primary action button, derived from the Finish control's state: Finish
  // while idle, busy while finishing or done, Retry once failed.
  finish: WizardFinishState
  createLabel?: string
  creatingLabel?: string
  onFinalize: () => void
  canFinalize?: boolean
  // Optional next button for multi-step wizards where step 0 is config.
  onNext?: () => void
  canNext?: boolean
  nextLabel?: string
  nextBusyLabel?: string
  nextBusy?: boolean
  // Which step shows the finalize button. Defaults to 0 (single-step).
  finalizeStep?: number
  // Long-form steps can opt into the established wide wizard width.
  width?: 'default' | 'wide'
}

type WizardNameFieldProps = Pick<
  WizardShellProps,
  | 'localName'
  | 'onUpdateName'
  | 'nameLabel'
  | 'namePlaceholder'
  | 'nameHelp'
  | 'nameError'
  | 'selectNameOnFocus'
>

/** WizardNameField renders the name input with its optional error message. */
function WizardNameField({
  localName,
  onUpdateName,
  nameLabel = 'Name',
  namePlaceholder = 'Enter a name...',
  nameHelp,
  nameError,
  selectNameOnFocus = false,
}: WizardNameFieldProps) {
  const handleNameInputRef = useCallback(
    (node: HTMLInputElement | null) => {
      node?.focus()
      if (selectNameOnFocus) node?.select()
    },
    [selectNameOnFocus],
  )
  const handleNameFocus = useCallback(
    (event: FocusEvent<HTMLInputElement>) => {
      if (selectNameOnFocus) event.currentTarget.select()
    },
    [selectNameOnFocus],
  )

  return (
    <section>
      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <WizardField
          inputRef={handleNameInputRef}
          label={nameLabel}
          value={localName}
          onChange={(e) => onUpdateName(e.target.value)}
          placeholder={namePlaceholder}
          help={nameHelp}
          aria-invalid={nameError ? true : undefined}
          aria-describedby={nameError ? 'wizard-name-error' : undefined}
          variant={nameError ? 'error' : 'default'}
          onFocus={handleNameFocus}
        />
        {nameError && (
          <p id="wizard-name-error" className="text-destructive mt-1.5 text-xs">
            {nameError}
          </p>
        )}
      </div>
    </section>
  )
}

/**
 * WizardFinishNotice says why the last Finish press did not finish. An input
 * the person can fix shows beside Finish; a failure keeps the wizard and the
 * input as they were, and Retry runs the Finish again.
 */
function WizardFinishNotice({ finish }: { finish: WizardFinishState }) {
  if (finish.status === 'idle' && finish.message) {
    return (
      <p role="alert" className="text-destructive text-xs">
        {finish.message}
      </p>
    )
  }
  if (finish.status !== 'failed') return null
  return (
    <ViewerStatus
      state="failed"
      message={`Could not finish: ${finish.error}`}
    />
  )
}

type WizardFooterProps = Pick<
  WizardShellProps,
  | 'step'
  | 'onBack'
  | 'canBack'
  | 'finish'
  | 'createLabel'
  | 'creatingLabel'
  | 'onFinalize'
  | 'canFinalize'
  | 'onNext'
  | 'canNext'
  | 'nextLabel'
  | 'nextBusyLabel'
  | 'nextBusy'
  | 'finalizeStep'
> & { canSubmit: boolean }

/** WizardFooter renders the back, next, and finalize buttons for the current step. */
function WizardFooter({
  step,
  onBack,
  canBack = true,
  finish,
  createLabel = 'Create',
  creatingLabel = 'Creating...',
  onFinalize,
  canFinalize = true,
  onNext,
  canNext = true,
  nextLabel = 'Next',
  nextBusyLabel = 'Opening…',
  nextBusy = false,
  finalizeStep,
  canSubmit,
}: WizardFooterProps) {
  // Finish shows on its step, or on every step when none is named.
  const showFinalize = finalizeStep === undefined || step === finalizeStep

  // Retry runs the whole Finish again on the kept input, so it needs the
  // same valid draft as Finish; Back waits while a Finish runs.
  const busy = finish.status === 'finishing' || finish.status === 'done'
  const retry = finish.status === 'failed'
  const finalizeLabel = busy ? creatingLabel : retry ? 'Retry' : createLabel

  return (
    <div className="border-foreground/8 flex items-center justify-between gap-2 border-t px-4 py-3">
      <div>
        {step > 0 && (
          <DashboardButton
            icon={<LuArrowLeft className="size-3.5" aria-hidden />}
            onClick={onBack}
            disabled={!canBack || busy}
          >
            Back
          </DashboardButton>
        )}
      </div>
      <div className="flex gap-2">
        {onNext && step < (finalizeStep ?? 0) && (
          <Button
            onClick={onNext}
            disabled={nextBusy || !canNext}
            variant="brandOutline"
            size="toolbarWide"
          >
            {nextBusy ? nextBusyLabel : nextLabel}
          </Button>
        )}
        {showFinalize && (
          <Button
            onClick={onFinalize}
            disabled={busy || !canSubmit || !canFinalize}
            variant="brandOutline"
            size="toolbarWide"
          >
            <LuCheck className="size-3.5" aria-hidden />
            {finalizeLabel}
          </Button>
        )}
      </div>
    </div>
  )
}

// WizardShell renders the shared wizard layout: header, step indicator,
// content slot, name input, and button grid. The content scrolls between the
// header and the buttons, so a short window keeps the buttons in reach.
export function WizardShell({
  icon,
  title,
  step,
  totalSteps,
  stepName,
  onCancel,
  cancel,
  children,
  nameStep = 0,
  width = 'default',
  ...rest
}: WizardShellProps) {
  // A wizard removed elsewhere can only be closed.
  if (rest.finish.status === 'gone') return <WizardGone />

  // Delete is offered only while this view's own Finish is not removing the
  // wizard; a Finish in another tab makes the delete refuse as gone.
  const deleteOffered =
    rest.finish.status !== 'finishing' && rest.finish.status !== 'done'

  const stepLabel =
    totalSteps !== undefined
      ? `Step ${step + 1} of ${totalSteps}`
      : `Step ${step + 1}`

  return (
    <div className="flex h-full w-full flex-col">
      <ViewerBar
        icon={icon}
        title={title}
        actions={
          deleteOffered
            ? [
                {
                  id: 'delete',
                  label:
                    cancel.status === 'cancelling'
                      ? 'Deleting wizard…'
                      : 'Delete wizard',
                  icon: <LuTrash2 />,
                  iconOnly: true,
                  variant: 'destructive',
                  disabled: cancel.status === 'cancelling',
                  onClick: onCancel,
                },
              ]
            : []
        }
      />
      {cancel.status === 'failed' && (
        <ViewerStatus
          state="failed"
          message={`Could not delete the wizard: ${cancel.error}`}
        />
      )}
      <div className="very-short:py-2 flex min-h-0 w-full flex-1 items-start justify-center px-4 py-10">
        <div
          className={cn(
            'border-foreground/6 bg-background-card/30 flex max-h-full w-full flex-col overflow-hidden rounded-xl border',
            width === 'wide' ? 'max-w-2xl' : 'max-w-lg',
          )}
        >
          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
            <div className="space-y-3">
              <div className="text-foreground-alt flex items-center gap-2 text-xs select-none">
                <span className="font-medium">{stepLabel}</span>
                {stepName && (
                  <span className="text-foreground text-xs font-medium">
                    {stepName}
                  </span>
                )}
              </div>

              {step === nameStep && <WizardNameField {...rest} />}

              {children}

              <WizardFinishNotice finish={rest.finish} />
            </div>
          </div>

          <WizardFooter
            step={step}
            canSubmit={!!rest.localName.trim()}
            {...rest}
          />
        </div>
      </div>
    </div>
  )
}

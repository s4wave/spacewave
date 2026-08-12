import { useCallback, type FocusEvent, type ReactNode } from 'react'
import { LuArrowLeft, LuCheck, LuTrash2 } from 'react-icons/lu'

import { Button } from '@s4wave/web/ui/button.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'
import { cn } from '@s4wave/web/style/utils.js'

import { WizardField } from './WizardField.js'

export interface WizardShellProps {
  title: ReactNode
  step: number
  totalSteps?: number
  stepName?: string
  localName: string
  onUpdateName: (name: string) => void
  onBack: () => void
  canBack?: boolean
  onCancel: () => void
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
  // Primary action button.
  creating: boolean
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

// WizardShell renders the shared wizard layout: header, step indicator,
// content slot, name input, and button grid.
export function WizardShell({
  title,
  step,
  totalSteps,
  stepName,
  localName,
  onUpdateName,
  onBack,
  canBack = true,
  onCancel,
  children,
  nameLabel = 'Name',
  namePlaceholder = 'Enter a name...',
  nameHelp,
  nameError,
  nameStep = 0,
  selectNameOnFocus = false,
  creating,
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
  width = 'default',
}: WizardShellProps) {
  const showFinalize = finalizeStep === undefined || step === finalizeStep
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
    <div className="flex h-full w-full items-start justify-center overflow-auto px-4 py-10">
      <div
        className={cn(
          'border-foreground/6 bg-background-card/30 flex w-full flex-col overflow-hidden rounded-xl border backdrop-blur-sm',
          width === 'wide' ? 'max-w-2xl' : 'max-w-lg',
        )}
      >
        <div className="border-foreground/8 flex h-9 shrink-0 items-center justify-between border-b px-4">
          <h2 className="text-foreground flex min-w-0 items-center text-sm font-semibold tracking-tight select-none">
            {title}
          </h2>
          <Tooltip>
            <TooltipTrigger asChild>
              <DashboardButton
                icon={<LuTrash2 className="size-3.5" />}
                onClick={onCancel}
                aria-label="Delete wizard"
                className="hover:border-destructive/30 hover:bg-destructive/5 hover:text-destructive"
              />
            </TooltipTrigger>
            <TooltipContent side="bottom">Delete wizard</TooltipContent>
          </Tooltip>
        </div>

        <div className="flex-1 px-4 py-3">
          <div className="space-y-3">
            <div className="text-foreground-alt/50 flex items-center gap-2 select-none">
              <span className="text-[0.6rem] font-medium tracking-widest uppercase">
                {totalSteps !== undefined
                  ? `Step ${step + 1} of ${totalSteps}`
                  : `Step ${step + 1}`}
              </span>
              {stepName && (
                <span className="text-foreground-alt/80 text-xs font-medium">
                  {stepName}
                </span>
              )}
            </div>

            {step === nameStep && (
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
                    aria-describedby={
                      nameError ? 'wizard-name-error' : undefined
                    }
                    className={nameError ? 'border-destructive/50' : undefined}
                    onFocus={handleNameFocus}
                  />
                  {nameError && (
                    <p
                      id="wizard-name-error"
                      className="text-destructive mt-1.5 text-xs"
                    >
                      {nameError}
                    </p>
                  )}
                </div>
              </section>
            )}

            {children}
          </div>
        </div>

        <div className="border-foreground/8 flex items-center justify-between gap-2 border-t px-4 py-3">
          <div>
            {step > 0 && (
              <DashboardButton
                icon={<LuArrowLeft className="size-3.5" />}
                onClick={onBack}
                disabled={!canBack}
              >
                Back
              </DashboardButton>
            )}
          </div>
          <div className="flex gap-2">
            {onNext && step < (finalizeStep ?? 0) && (
              <Button
                size="sm"
                onClick={onNext}
                disabled={nextBusy || !canNext}
                className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground h-7 rounded-md border px-3 text-xs transition duration-150"
              >
                {nextBusy ? nextBusyLabel : nextLabel}
              </Button>
            )}
            {showFinalize && (
              <Button
                size="sm"
                onClick={onFinalize}
                disabled={creating || !localName.trim() || !canFinalize}
                className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground h-7 rounded-md border px-3 text-xs transition duration-150"
              >
                <LuCheck className="size-3.5" />
                {creating ? creatingLabel : createLabel}
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

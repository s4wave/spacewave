import type { ReactNode } from 'react'
import { LuArrowLeft, LuCheck, LuTrash2 } from 'react-icons/lu'

import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

export interface WizardShellProps {
  title: ReactNode
  step: number
  totalSteps?: number
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
  // Which step shows the name input. Defaults to 0.
  nameStep?: number
  // Primary action button.
  creating: boolean
  createLabel?: string
  creatingLabel?: string
  onFinalize: () => void
  canFinalize?: boolean
  // Optional next button for multi-step wizards where step 0 is config.
  onNext?: () => void
  canNext?: boolean
  // Which step shows the finalize button. Defaults to 0 (single-step).
  finalizeStep?: number
}

// WizardShell renders the shared wizard layout: header, step indicator,
// content slot, name input, and button grid.
export function WizardShell({
  title,
  step,
  totalSteps,
  localName,
  onUpdateName,
  onBack,
  canBack = true,
  onCancel,
  children,
  nameLabel = 'Name',
  namePlaceholder = 'Enter a name...',
  nameHelp,
  nameStep = 0,
  creating,
  createLabel = 'Create',
  creatingLabel = 'Creating...',
  onFinalize,
  canFinalize = true,
  onNext,
  canNext = true,
  finalizeStep,
}: WizardShellProps) {
  const showFinalize = finalizeStep === undefined || step === finalizeStep

  return (
    <div className="flex h-full w-full items-start justify-center overflow-auto px-4 py-10">
      <div className="border-foreground/6 bg-background-card/30 flex w-full max-w-lg flex-col overflow-hidden rounded-xl border backdrop-blur-sm">
        <div className="border-foreground/8 flex h-9 shrink-0 items-center justify-between border-b px-4">
          <h2 className="text-foreground flex min-w-0 items-center text-sm font-semibold tracking-tight select-none">
            {title}
          </h2>
          <Tooltip>
            <TooltipTrigger asChild>
              <DashboardButton
                icon={<LuTrash2 className="h-3.5 w-3.5" />}
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
            <div className="text-foreground-alt/50 flex items-center text-[0.6rem] font-medium tracking-widest uppercase select-none">
              {totalSteps !== undefined ?
                `Step ${step + 1} of ${totalSteps}`
              : `Step ${step + 1}`}
            </div>

            {step === nameStep && (
              <section>
                <div className="mb-2 flex items-center justify-between">
                  <label className="text-foreground text-xs font-medium select-none">
                    {nameLabel}
                  </label>
                </div>
                <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
                  <Input
                    value={localName}
                    onChange={(e) => onUpdateName(e.target.value)}
                    placeholder={namePlaceholder}
                    autoFocus
                    className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9"
                  />
                  {nameHelp && (
                    <p className="text-foreground-alt/50 mt-2 text-xs">
                      {nameHelp}
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
                icon={<LuArrowLeft className="h-3.5 w-3.5" />}
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
                disabled={!canNext}
                className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground h-7 rounded-md border px-3 text-xs transition-all duration-150"
              >
                Next
              </Button>
            )}
            {showFinalize && (
              <Button
                size="sm"
                onClick={onFinalize}
                disabled={creating || !localName.trim() || !canFinalize}
                className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground h-7 rounded-md border px-3 text-xs transition-all duration-150"
              >
                <LuCheck className="h-3.5 w-3.5" />
                {creating ? creatingLabel : createLabel}
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

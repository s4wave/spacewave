import { LuCircleCheck } from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'

export interface PhaseChecklistItem {
  label: string
  done: boolean
  active?: boolean
}

export interface PhaseChecklistProps {
  phases: PhaseChecklistItem[]
  className?: string
}

// PhaseChecklist renders an ordered list of phases as a vertical checklist
// with a check, spinner, or pending dot per item.
export function PhaseChecklist({ phases, className }: PhaseChecklistProps) {
  return (
    <div className={cn('space-y-2 px-2', className)}>
      {phases.map((phase) => (
        <PhaseChecklistRow key={phase.label} {...phase} />
      ))}
    </div>
  )
}

function PhaseChecklistRow({ label, done, active }: PhaseChecklistItem) {
  return (
    <div className="flex items-center gap-2">
      {done ?
        <LuCircleCheck className="text-brand size-4" />
      : active ?
        <Spinner className="text-brand" />
      : <div className="border-foreground/20 size-4 rounded-full border" />}
      <span
        className={cn(
          'text-xs',
          done ? 'text-foreground' : 'text-foreground-alt',
        )}
      >
        {label}
      </span>
    </div>
  )
}

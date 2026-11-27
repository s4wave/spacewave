import { Badge } from '@s4wave/web/ui/badge.js'
import { VmState } from '@s4wave/sdk/vm/v86.pb.js'

// stateBadge labels each observed VM state and picks its badge treatment.
const stateBadge: Record<
  VmState,
  { label: string; variant: 'muted' | 'subtle' | 'success' | 'conflict' }
> = {
  [VmState.VmState_STOPPED]: { label: 'Stopped', variant: 'muted' },
  [VmState.VmState_STARTING]: { label: 'Starting', variant: 'subtle' },
  [VmState.VmState_RUNNING]: { label: 'Running', variant: 'success' },
  [VmState.VmState_STOPPING]: { label: 'Stopping', variant: 'subtle' },
  [VmState.VmState_ERROR]: { label: 'Error', variant: 'conflict' },
}

// VmStateBadge shows the observed state of a VM as a chip. A starting or
// stopping VM pulses its dot.
export function VmStateBadge({ state }: { state: VmState }) {
  const { label, variant } = stateBadge[state]
  return (
    <Badge variant={variant} data-state={label} className="group">
      <span
        aria-hidden
        className="size-1.5 rounded-full bg-current group-data-[state=Starting]:animate-pulse group-data-[state=Stopping]:animate-pulse"
      />
      {label}
    </Badge>
  )
}

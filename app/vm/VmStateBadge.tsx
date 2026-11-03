import { keyframeNames } from '../../web/style/tokens.stylex.js'
import { Badge } from '@s4wave/web/ui/badge.js'
import { VmState } from '@s4wave/sdk/vm/v86.pb.js'

import * as stylex from '@stylexjs/stylex'
import { groupMarker } from '../../web/style/relations.stylex.js'

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const styles = stylex.create({
  rule1: {
    borderRadius: '3.40282e38px',
    animationDelay: {
      default: null,
      [stylex.when.ancestor(':is([data-state="Starting"])', groupMarker)]: '0s',
      [stylex.when.ancestor(':is([data-state="Stopping"])', groupMarker)]: '0s',
    },
    animationDirection: {
      default: null,
      [stylex.when.ancestor(':is([data-state="Starting"])', groupMarker)]:
        'normal',
      [stylex.when.ancestor(':is([data-state="Stopping"])', groupMarker)]:
        'normal',
    },
    animationDuration: {
      default: null,
      [stylex.when.ancestor(':is([data-state="Starting"])', groupMarker)]: '2s',
      [stylex.when.ancestor(':is([data-state="Stopping"])', groupMarker)]: '2s',
    },
    animationFillMode: {
      default: null,
      [stylex.when.ancestor(':is([data-state="Starting"])', groupMarker)]:
        'none',
      [stylex.when.ancestor(':is([data-state="Stopping"])', groupMarker)]:
        'none',
    },
    animationIterationCount: {
      default: null,
      [stylex.when.ancestor(':is([data-state="Starting"])', groupMarker)]:
        'infinite',
      [stylex.when.ancestor(':is([data-state="Stopping"])', groupMarker)]:
        'infinite',
    },
    animationName: {
      default: null,
      [stylex.when.ancestor(':is([data-state="Starting"])', groupMarker)]:
        keyframeNames['pulse'],
      [stylex.when.ancestor(':is([data-state="Stopping"])', groupMarker)]:
        keyframeNames['pulse'],
    },
    animationPlayState: {
      default: null,
      [stylex.when.ancestor(':is([data-state="Starting"])', groupMarker)]:
        'running',
      [stylex.when.ancestor(':is([data-state="Stopping"])', groupMarker)]:
        'running',
    },
    animationTimeline: {
      default: null,
      [stylex.when.ancestor(':is([data-state="Starting"])', groupMarker)]:
        'auto',
      [stylex.when.ancestor(':is([data-state="Stopping"])', groupMarker)]:
        'auto',
    },
    animationTimingFunction: {
      default: null,
      [stylex.when.ancestor(':is([data-state="Starting"])', groupMarker)]:
        'cubic-bezier(.4, 0, .6, 1)',
      [stylex.when.ancestor(':is([data-state="Stopping"])', groupMarker)]:
        'cubic-bezier(.4, 0, .6, 1)',
    },
    backgroundColor: 'currentColor',
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
})

// stateBadge names each observed VM state and picks its badge treatment.
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
    <Badge variant={variant} data-state={label} sx={groupMarker}>
      <span aria-hidden sx={styles.rule1} />
      {label}
    </Badge>
  )
}

import type { SystemTone } from './useSystemModel.js'

// toneDotClass is the indicator fill for each system tone.
export const toneDotClass: Record<SystemTone, string> = {
  nominal: 'bg-success',
  active: 'bg-brand',
  warning: 'bg-warning',
  error: 'bg-destructive',
  pending: 'bg-foreground-alt/30',
}

// toneTextClass is the text color that carries each system tone.
export const toneTextClass: Record<SystemTone, string> = {
  nominal: 'text-success',
  active: 'text-brand',
  warning: 'text-warning',
  error: 'text-destructive',
  pending: 'text-foreground-alt/60',
}

// toneLabel names each system tone for assistive technology.
export const toneLabel: Record<SystemTone, string> = {
  nominal: 'Nominal',
  active: 'Working',
  warning: 'Needs attention',
  error: 'Fault',
  pending: 'Reading',
}

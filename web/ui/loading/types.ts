// LoadingState is the four-state machine shared by every loading surface.
export type LoadingState = 'loading' | 'active' | 'synced' | 'error'

// LoadingRate describes optional transfer rate pills attached to a LoadingView.
export interface LoadingRate {
  up?: string
  down?: string
}

// LoadingView is the contract between adapters and primitives. Adapters in
// app/loading/status/* produce these views; primitives in web/ui/loading/*
// consume them. Optional fields render nothing when absent.
export interface LoadingView {
  state: LoadingState
  title: string
  detail?: string
  // progress is in 0..1.
  progress?: number
  // progressIndeterminate shows a sweeping progress bar when byte progress is
  // unknown.
  progressIndeterminate?: boolean
  rate?: LoadingRate
  lastActivity?: string
  error?: string
  onRetry?: () => void
  retryLabel?: string
  cancelLabel?: string
  onCancel?: () => void
}

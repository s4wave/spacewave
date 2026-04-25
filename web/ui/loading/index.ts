// Unified loading primitive family. Adapters in app/loading/status/* produce
// LoadingView values; the primitives here consume them. See
// guides/alpha-ui-design-system.org "Loading" section for the four-state
// contract and domain wrapper pattern.

export { LoadingCard } from './LoadingCard.js'
export { LoadingInline, type LoadingInlineTone } from './LoadingInline.js'
export { LoadingScreen } from './LoadingScreen.js'
export { ProgressBar } from './ProgressBar.js'
export { Spinner, type SpinnerSize } from './Spinner.js'
export type { LoadingRate, LoadingState, LoadingView } from './types.js'

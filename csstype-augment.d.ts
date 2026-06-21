// Augments csstype so React style objects accept the non-standard
// WebkitAppRegion property used for draggable native window chrome.
//
// This must live in a module file (the top-level import below makes it one) so
// `declare module 'csstype'` is a module augmentation. The same block in a
// global script file is parsed as an ambient module declaration that replaces
// csstype entirely, hiding Properties and breaking every CSSProperties key.
//
// The type parameters must stay identical to csstype's own
// StandardLonghandProperties declaration, or interface merging is rejected.
import 'csstype'
import 'react'

declare module 'csstype' {
  interface StandardLonghandProperties<TLength = (string & {}) | 0, TTime = string & {}> {
    WebkitAppRegion?: 'drag' | 'no-drag' | string
  }
}

// React's CSSProperties uses closed csstype typing with no index signature, so
// inline CSS custom properties (--foo) are rejected. Re-admit only --prefixed
// keys; standard property typos still fail.
declare module 'react' {
  interface CSSProperties {
    [customProperty: `--${string}`]: string | number | undefined
  }
}

import type { StyleXStyles } from '@stylexjs/stylex'

// The compiler lowers intrinsic sx attributes before React receives their props.
declare module 'react' {
  interface HTMLAttributes<T> {
    sx?: StyleXStyles
  }

  interface SVGAttributes<T> {
    sx?: StyleXStyles
  }
}

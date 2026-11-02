import * as stylex from '@stylexjs/stylex'
import type { CSSProperties } from 'react'

/** stylexProps preserves compiled classes and variables beside native caller properties. */
export function stylexProps<
  Props extends { style?: CSSProperties; className?: never },
>(
  styles: stylex.StyleXArray<
    | stylex.CompiledStyles
    | Readonly<[stylex.CompiledStyles, stylex.InlineStyles]>
    | boolean
    | null
    | undefined
  >,
  props: Props,
) {
  // Compile the owned and caller sx values through the official runtime.
  const compiled = stylex.props(styles)
  const nativeProps: Omit<Props, 'className' | 'style'> = props

  // Native inline overrides retain the variables emitted for dynamic sx values.
  return {
    ...nativeProps,
    ...compiled,
    style: props.style ? { ...compiled.style, ...props.style } : compiled.style,
  }
}

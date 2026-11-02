import { createContext, use, useMemo, type ReactNode } from 'react'
import * as stylex from '@stylexjs/stylex'

// IconStyleScope separates overridable defaults from independently scoped control rules.
interface IconStyleScope {
  sx?: stylex.StyleXStyles
  required: readonly ReturnType<typeof stylex.props>[]
}
const IconStyleContext = createContext<IconStyleScope>({ required: [] })

/** IconStylesProvider gives descendant icon owners defaults and DOM-scoped control rules. */
export function IconStylesProvider({
  sx,
  requiredSx,
  children,
}: {
  sx?: stylex.StyleXStyles
  requiredSx?: stylex.StyleXStyles
  children: ReactNode
}) {
  const inherited = use(IconStyleContext)
  const value = useMemo(
    () => ({
      sx: [inherited.sx, sx],
      required: requiredSx
        ? [...inherited.required, stylex.props(requiredSx)]
        : inherited.required,
    }),
    [inherited, sx, requiredSx],
  )
  return (
    <IconStyleContext.Provider value={value}>
      {children}
    </IconStyleContext.Provider>
  )
}

/** useIconStyles reads the defaults and scoped rules owned by the enclosing controls. */
export function useIconStyles(): IconStyleScope {
  return use(IconStyleContext)
}

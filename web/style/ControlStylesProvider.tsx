import { createContext, use, type ReactNode } from 'react'
import * as stylex from '@stylexjs/stylex'

/** controlTargetStyles gives native controls the shared pointer and touch target floors. */
export const controlTargetStyles = stylex.create({
  minimum: {
    minHeight: { default: '24px', '@media (pointer: coarse)': '44px' },
    minWidth: { default: '24px', '@media (pointer: coarse)': '44px' },
  },
})

const ControlStyleContext = createContext<string | undefined>(undefined)

/** ControlStylesProvider carries compiled, DOM-gated rules to the controls that render their classes. */
export function ControlStylesProvider({
  styles,
  children,
}: {
  styles: stylex.CompiledStyles | readonly stylex.CompiledStyles[]
  children: ReactNode
}) {
  const inherited = use(ControlStyleContext)
  const owned = stylex.props(styles).className
  const classes =
    inherited === owned ? owned : [inherited, owned].filter(Boolean).join(' ')
  return (
    <ControlStyleContext.Provider value={classes}>
      {children}
    </ControlStyleContext.Provider>
  )
}

/** useControlStyles reads independently composed rules without clearing a control's variant defaults. */
export function useControlStyles(): string | undefined {
  return use(ControlStyleContext)
}

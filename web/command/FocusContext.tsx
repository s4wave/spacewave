import { createContext, use, useMemo, type ReactNode } from 'react'
import { CommandFocusContext } from '@s4wave/sdk/command/command.pb.js'

import { useIsTabActive } from '@s4wave/web/contexts/TabActiveContext.js'

const focusContextAttribute = 'data-command-focus-context'
const rootFocusContextStack: CommandFocusContext[] = [
  CommandFocusContext.GLOBAL,
]

const FocusContextStackContext = createContext<readonly CommandFocusContext[]>(
  rootFocusContextStack,
)

export interface FocusContextProviderProps {
  focusContext: CommandFocusContext
  active?: boolean
  className?: string
  children?: ReactNode
}

export function FocusContextProvider({
  focusContext,
  active = true,
  className,
  children,
}: FocusContextProviderProps) {
  const parentStack = use(FocusContextStackContext)
  const stack = useMemo(
    () => appendFocusContext(parentStack, active ? focusContext : undefined),
    [parentStack, active, focusContext],
  )

  if (!active) return <>{children ?? null}</>

  return (
    <FocusContextStackContext.Provider value={stack}>
      <div className={className} {...focusContextDomProps(focusContext)}>
        {children ?? null}
      </div>
    </FocusContextStackContext.Provider>
  )
}

export interface FocusContextStackProviderProps {
  focusContext: CommandFocusContext
  active?: boolean
  children?: ReactNode
}

export function FocusContextStackProvider({
  focusContext,
  active = true,
  children,
}: FocusContextStackProviderProps) {
  const parentStack = use(FocusContextStackContext)
  const stack = useMemo(
    () => appendFocusContext(parentStack, active ? focusContext : undefined),
    [parentStack, active, focusContext],
  )

  return (
    <FocusContextStackContext.Provider value={stack}>
      {children ?? null}
    </FocusContextStackContext.Provider>
  )
}

export function ShellTabFocusContextProvider({
  children,
}: {
  children?: ReactNode
}) {
  const isTabActive = useIsTabActive()
  return (
    <FocusContextStackProvider
      focusContext={CommandFocusContext.SHELL_TAB}
      active={isTabActive}
    >
      {children ?? null}
    </FocusContextStackProvider>
  )
}

export function useFocusContextStack(): readonly CommandFocusContext[] {
  return use(FocusContextStackContext)
}

export function useFocusContextResolver(): (
  target: EventTarget | null,
) => readonly CommandFocusContext[] {
  const baseStack = useFocusContextStack()
  return useMemo(
    () => (target: EventTarget | null) =>
      resolveFocusContextsForTarget(target, baseStack),
    [baseStack],
  )
}

export function resolveFocusContextsForTarget(
  target: EventTarget | null,
  baseStack: readonly CommandFocusContext[] = rootFocusContextStack,
): readonly CommandFocusContext[] {
  const stack = [...normalizeFocusContextStack(baseStack)]
  if (target instanceof HTMLElement) {
    for (const context of readDomFocusContexts(target)) {
      appendFocusContextInPlace(stack, context)
    }
    if (
      isTextInputTarget(target) &&
      !stack.includes(CommandFocusContext.EDITOR)
    ) {
      appendFocusContextInPlace(stack, CommandFocusContext.TEXT_INPUT)
    }
  }
  return stack
}

export function focusContextDomProps(focusContext: CommandFocusContext): {
  [focusContextAttribute]: string
} {
  return { [focusContextAttribute]: String(focusContext) }
}

function appendFocusContext(
  stack: readonly CommandFocusContext[],
  focusContext: CommandFocusContext | undefined,
): readonly CommandFocusContext[] {
  const next = [...normalizeFocusContextStack(stack)]
  appendFocusContextInPlace(next, focusContext)
  return next
}

function appendFocusContextInPlace(
  stack: CommandFocusContext[],
  focusContext: CommandFocusContext | undefined,
): void {
  if (
    focusContext == null ||
    focusContext === CommandFocusContext.UNSPECIFIED ||
    stack.includes(focusContext)
  ) {
    return
  }
  stack.push(focusContext)
}

function normalizeFocusContextStack(
  stack: readonly CommandFocusContext[],
): readonly CommandFocusContext[] {
  const normalized: CommandFocusContext[] = []
  appendFocusContextInPlace(normalized, CommandFocusContext.GLOBAL)
  for (const context of stack) appendFocusContextInPlace(normalized, context)
  return normalized
}

function readDomFocusContexts(target: HTMLElement): CommandFocusContext[] {
  const contexts: CommandFocusContext[] = []
  let node: HTMLElement | null = target
  while (node) {
    const raw = node.getAttribute(focusContextAttribute)
    if (raw != null) {
      const context = Number(raw)
      if (Number.isFinite(context)) contexts.unshift(context)
    }
    node = node.parentElement
  }
  return contexts
}

function isTextInputTarget(target: HTMLElement): boolean {
  return (
    target.tagName === 'INPUT' ||
    target.tagName === 'TEXTAREA' ||
    target.isContentEditable
  )
}

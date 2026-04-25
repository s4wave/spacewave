import { createContext, useContext, useMemo, type ReactNode } from 'react'
import type { RuntimeHandoffState } from '@s4wave/sdk/root/root.pb.js'

// RuntimeHandoffContextValue carries the current handoff snapshot so
// components across the app can detect when the native runtime has
// been yielded to a remote CLI daemon. Components that depend on the
// runtime (e.g. buttons that launch a web listener, start a transfer,
// or mount a shared object) should gate user actions on this.
export interface RuntimeHandoffContextValue {
  // active is true when the desktop listener has yielded the socket
  // and the runtime is owned by the requesting peer.
  active: boolean
  // requesterName is the display name of the runtime that took over.
  requesterName: string
  // socketPath is the path that was yielded.
  socketPath: string
}

const defaultValue: RuntimeHandoffContextValue = {
  active: false,
  requesterName: '',
  socketPath: '',
}

const RuntimeHandoffContext =
  createContext<RuntimeHandoffContextValue>(defaultValue)

// RuntimeHandoffProvider publishes the handoff state to descendants.
export function RuntimeHandoffProvider({
  state,
  children,
}: {
  state: RuntimeHandoffState | null
  children: ReactNode
}) {
  const value: RuntimeHandoffContextValue = useMemo(() => {
    if (!state?.active) {
      return defaultValue
    }
    return {
      active: true,
      requesterName: state.requesterName ?? '',
      socketPath: state.socketPath ?? '',
    }
  }, [state])
  return (
    <RuntimeHandoffContext.Provider value={value}>
      {children}
    </RuntimeHandoffContext.Provider>
  )
}

// useRuntimeHandoff returns the current runtime-handoff state.
// Components that depend on the native runtime can disable actions or
// show copy when handoff.active is true.
export function useRuntimeHandoff(): RuntimeHandoffContextValue {
  return useContext(RuntimeHandoffContext)
}

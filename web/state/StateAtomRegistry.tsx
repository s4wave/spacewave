import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from 'react'

export interface InspectableStateAtom {
  get(): unknown
  subscribe(callback: () => void): () => void
}

export type StateAtomScope = 'local' | 'persistent'

export interface StateAtomEntry {
  id: string
  name: string
  scope: StateAtomScope
  atom: InspectableStateAtom
}

export interface StateAtomRegistryContextValue {
  registerAtom: (
    id: string,
    name: string,
    scope: StateAtomScope,
    atom: InspectableStateAtom,
  ) => void
  unregisterAtom: (id: string) => void
  subscribe: (callback: () => void) => () => void
  subscribeSelectedAtom: (callback: () => void) => () => void
  getAtoms: () => Map<string, StateAtomEntry>
  getSelectedAtomId: () => string | null
  setSelectedAtomId: (id: string | null) => void
  getSelectedPath: () => string[]
  setSelectedPath: (path: string[]) => void
}

const StateAtomRegistryContext =
  createContext<StateAtomRegistryContextValue | null>(null)

const EMPTY_ATOMS_MAP = new Map<string, StateAtomEntry>()
const EMPTY_SELECTED_PATH: string[] = []

export function StateAtomRegistryProvider({
  children,
  enabled = true,
}: {
  children: ReactNode
  enabled?: boolean
}) {
  const [atoms, setAtoms] = useState(() => new Map<string, StateAtomEntry>())
  const [selectedAtomId, setSelectedAtomIdState] = useState<string | null>(null)
  const [selectedPath, setSelectedPathState] = useState<string[]>([])

  const atomsRef = useRef(atoms)
  const atomRefCounts = useRef(new Map<string, number>())
  useEffect(() => {
    atomsRef.current = atoms
  }, [atoms])

  const subscribersRef = useRef(new Set<() => void>())
  const subscribe = useCallback((callback: () => void) => {
    subscribersRef.current.add(callback)
    return () => subscribersRef.current.delete(callback)
  }, [])

  const notifySubscribers = useCallback(() => {
    subscribersRef.current.forEach((cb) => cb())
  }, [])

  useEffect(() => {
    notifySubscribers()
  }, [atoms, notifySubscribers])

  const selectedAtomSubscribersRef = useRef(new Set<() => void>())
  const subscribeSelectedAtom = useCallback((callback: () => void) => {
    selectedAtomSubscribersRef.current.add(callback)
    return () => selectedAtomSubscribersRef.current.delete(callback)
  }, [])

  const notifySelectedAtomSubscribers = useCallback(() => {
    selectedAtomSubscribersRef.current.forEach((cb) => cb())
  }, [])

  const selectedAtomIdRef = useRef(selectedAtomId)
  const selectedPathRef = useRef(selectedPath)
  useEffect(() => {
    selectedAtomIdRef.current = selectedAtomId
    selectedPathRef.current = selectedPath
    notifySelectedAtomSubscribers()
  }, [selectedAtomId, selectedPath, notifySelectedAtomSubscribers])

  const setSelectedAtomId = useCallback((id: string | null) => {
    setSelectedAtomIdState(id)
    setSelectedPathState([])
  }, [])

  const setSelectedPath = useCallback((path: string[]) => {
    setSelectedPathState(path)
  }, [])

  const registerAtom = useCallback(
    (
      id: string,
      name: string,
      scope: StateAtomScope,
      atom: InspectableStateAtom,
    ) => {
      atomRefCounts.current.set(id, (atomRefCounts.current.get(id) ?? 0) + 1)
      setAtoms((prev) => {
        const existing = prev.get(id)
        if (
          existing?.name === name &&
          existing.scope === scope &&
          existing.atom === atom
        ) {
          return prev
        }
        const next = new Map(prev)
        next.set(id, { id, name, scope, atom })
        return next
      })
    },
    [],
  )

  const unregisterAtom = useCallback((id: string) => {
    const refCount = atomRefCounts.current.get(id) ?? 0
    if (refCount > 1) {
      atomRefCounts.current.set(id, refCount - 1)
      return
    }

    atomRefCounts.current.delete(id)
    setAtoms((prev) => {
      if (!prev.has(id)) return prev
      const next = new Map(prev)
      next.delete(id)
      return next
    })
    setSelectedAtomIdState((prev) => (prev === id ? null : prev))
  }, [])

  const getAtoms = useCallback(() => atomsRef.current, [])

  const contextValue = useMemo(
    () => ({
      registerAtom,
      unregisterAtom,
      subscribe,
      subscribeSelectedAtom,
      getAtoms,
      getSelectedAtomId: () => selectedAtomIdRef.current,
      setSelectedAtomId,
      getSelectedPath: () => selectedPathRef.current,
      setSelectedPath,
    }),
    [
      registerAtom,
      unregisterAtom,
      subscribe,
      subscribeSelectedAtom,
      getAtoms,
      setSelectedAtomId,
      setSelectedPath,
    ],
  )

  return (
    <StateAtomRegistryContext.Provider value={enabled ? contextValue : null}>
      {children}
    </StateAtomRegistryContext.Provider>
  )
}

export function useStateAtomRegistryContext() {
  return useContext(StateAtomRegistryContext)
}

export function useStateAtoms(): Map<string, StateAtomEntry> {
  const registry = useStateAtomRegistryContext()

  const subscribe = useCallback(
    (callback: () => void) => {
      if (!registry) return () => {}
      return registry.subscribe(callback)
    },
    [registry],
  )

  const getSnapshot = useCallback(() => {
    if (!registry) return EMPTY_ATOMS_MAP
    return registry.getAtoms()
  }, [registry])

  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

export function useSelectedStateAtomId(): string | null {
  const registry = useStateAtomRegistryContext()

  const subscribe = useCallback(
    (callback: () => void) => {
      if (!registry) return () => {}
      return registry.subscribeSelectedAtom(callback)
    },
    [registry],
  )

  const getSnapshot = useCallback(() => {
    if (!registry) return null
    return registry.getSelectedAtomId()
  }, [registry])

  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

export function useSelectedStatePath(): string[] {
  const registry = useStateAtomRegistryContext()

  const subscribe = useCallback(
    (callback: () => void) => {
      if (!registry) return () => {}
      return registry.subscribeSelectedAtom(callback)
    },
    [registry],
  )

  const getSnapshot = useCallback(() => {
    if (!registry) return EMPTY_SELECTED_PATH
    return registry.getSelectedPath()
  }, [registry])

  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

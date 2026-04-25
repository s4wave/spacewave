import React, {
  createContext,
  useContext,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useSyncExternalStore,
  type ReactNode,
} from 'react'
import superjson, { type SuperJSONResult } from 'superjson'
import isDeepEqual from 'lodash.isequal'
import { useLatestRef } from '@aptre/bldr-react'
import type { StateAtomAccessor } from './useBackendStateAtom.js'
import { useBackendStateAtom } from './useBackendStateAtom.js'
import {
  useStateAtomRegistryContext,
  type StateAtomScope,
  type InspectableStateAtom,
} from './StateAtomRegistry.js'

// Stable empty resource used when no backend accessor is available.
const nullAccessor: StateAtomAccessor = {
  value: null,
  loading: false,
  error: null,
  retry: () => {},
}

// Storage interface for persistence
export interface Storage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

export class LocalStorageImpl implements Storage {
  getItem(key: string): string | null {
    return localStorage.getItem(key)
  }

  setItem(key: string, value: string): void {
    localStorage.setItem(key, value)
  }

  removeItem(key: string): void {
    localStorage.removeItem(key)
  }
}

// Atom interface
export interface Atom<T> {
  get(): T
  set: (value: T) => void
  subscribe(callback: () => void): () => void
}

// Base implementation of an Atom
class BasicAtom<T> implements Atom<T> {
  protected _value: T
  protected listeners: Set<() => void>

  constructor(initialValue: T) {
    this._value = initialValue
    this.listeners = new Set()
  }

  get(): T {
    return this._value
  }

  set = (value: T): void => {
    this._value = value
    this.notify()
  }

  subscribe(callback: () => void): () => void {
    this.listeners.add(callback)
    return () => this.listeners.delete(callback)
  }

  protected notify(): void {
    this.listeners.forEach((listener) => listener())
  }
}

// Storage-backed Atom implementation with cross-window sync
export class StorageAtom<T> extends BasicAtom<T> {
  private storage: Storage
  private key: string
  private storageListener?: (e: StorageEvent) => void

  constructor(storage: Storage, key: string, initialValue: T) {
    super(initialValue)
    this.storage = storage
    this.key = key

    // Load initial state
    try {
      const stored = storage.getItem(key)
      if (stored) {
        const value = superjson.deserialize(
          JSON.parse(stored) as SuperJSONResult,
        )
        this._value = value as T
      }
    } catch (e) {
      console.error('Failed to load persisted state:', e)
    }

    // Listen for cross-window storage changes
    if (
      typeof window !== 'undefined' &&
      typeof window.addEventListener === 'function'
    ) {
      this.storageListener = (e: StorageEvent) => {
        if (e.key !== this.key) return
        try {
          if (e.newValue === null) {
            this._value = initialValue
          } else {
            this._value = superjson.deserialize(
              JSON.parse(e.newValue) as SuperJSONResult,
            )
          }
          this.notify()
        } catch (err) {
          console.error('Failed to sync cross-window state:', err)
        }
      }
      window.addEventListener('storage', this.storageListener)
    }
  }

  set = (value: T): void => {
    this._value = value
    try {
      if (
        value === null ||
        value === undefined ||
        (typeof value === 'object' && Object.keys(value).length === 0)
      ) {
        this.storage.removeItem(this.key)
      } else {
        this.storage.setItem(
          this.key,
          JSON.stringify(superjson.serialize(value)),
        )
      }
    } catch (e) {
      console.error('Failed to persist state:', e)
    }
    this.notify()
  }

  // dispose removes the storage event listener
  dispose(): void {
    if (this.storageListener) {
      window.removeEventListener('storage', this.storageListener)
      this.storageListener = undefined
    }
  }
}

// Derived Atom implementation
export class DerivedAtom<T, U> implements Atom<U> {
  private source: Atom<T>
  private deriveFn: (value: T) => U
  private listeners: Set<() => void> = new Set()

  constructor(source: Atom<T>, deriveFn: (value: T) => U) {
    this.source = source
    this.deriveFn = deriveFn
  }

  get(): U {
    return this.deriveFn(this.source.get())
  }

  set = (_value: U): void => {
    throw new Error('Cannot set a derived atom directly')
  }

  subscribe(callback: () => void): () => void {
    this.listeners.add(callback)
    const unsubSource = this.source.subscribe(() => {
      callback()
    })
    return () => {
      this.listeners.delete(callback)
      unsubSource()
    }
  }
}

// useMemoPath memoizes a path array by comparing string contents rather than reference.
export function useMemoPath(path: string[] | null | undefined): string[] {
  const pathStr = path?.join('\0') ?? ''
  // eslint-disable-next-line react-hooks/preserve-manual-memoization, react-hooks/exhaustive-deps
  return useMemo(() => path ?? [], [pathStr])
}

/**
 * Context type for managing namespaced state
 */
export type StateType = Record<string, unknown>

export const StateNamespaceContext = createContext<StateNamespace | undefined>(
  undefined,
)

/**
 * Provider component for managing namespaced state
 * @param {Object} props - Component props
 * @param {ReactNode} props.children - Child components
 * @param {string[]} [props.namespace] - Optional namespace identifier
 * @param {StateNamespace} [props.stateNamespace] - Optional state namespace to inherit path from
 * @param {ReturnType<typeof atomWithStorage<StateType>>} [props.rootAtom] - Optional root storage atom
 */
export function StateNamespaceProvider({
  children,
  namespace: namespaceProp,
  rootAtom,
  stateNamespace,
  stateAtomAccessor,
}: {
  children?: ReactNode
  namespace?: string[]
  rootAtom?: Atom<StateType>
  stateNamespace?: StateNamespace
  stateAtomAccessor?: StateAtomAccessor
}) {
  const parentContext = useContext(StateNamespaceContext)
  const inheritedNamespace = stateNamespace?.namespace
  const namespace = useMemoPath(inheritedNamespace ?? namespaceProp)
  const contextValue = useMemo(() => {
    const parentNamespace = parentContext?.namespace ?? []
    const childNamespace =
      inheritedNamespace ? namespace
      : namespace.length > 0 ? [...parentNamespace, ...namespace]
      : parentNamespace

    return {
      namespace: childNamespace,
      stateAtom: rootAtom ?? parentContext?.stateAtom ?? atom<StateType>({}),
      stateAtomAccessor: stateAtomAccessor ?? parentContext?.stateAtomAccessor,
    }
  }, [
    inheritedNamespace,
    rootAtom,
    namespace,
    parentContext?.namespace,
    parentContext?.stateAtom,
    parentContext?.stateAtomAccessor,
    stateAtomAccessor,
  ])

  return (
    <StateNamespaceContext.Provider value={contextValue}>
      {children}
    </StateNamespaceContext.Provider>
  )
}

/**
 * Type representing a state namespace
 */
export type StateNamespace = {
  namespace: string[]
  stateAtom: Atom<StateType>
  stateAtomAccessor?: StateAtomAccessor
}

// Re-export StateAtomAccessor type from the backend atom hook.
export type { StateAtomAccessor } from './useBackendStateAtom.js'

/**
 * Hook to create a namespace by combining current context with segments and/or partial namespace
 * @param {string[]} [segments] - Additional path segments to append to context path
 * @param {Partial<StateNamespace>} [partial] - Partial namespace to override context
 * @returns {StateNamespace} Combined namespace
 */
export function useStateNamespace(
  segments?: string[],
  partial?: Partial<StateNamespace>,
): StateNamespace {
  const context = useContext(StateNamespaceContext)
  const contextNamespace = context?.namespace ?? []
  const segmentPath = segments ?? []

  // If partial.namespace exists, it overrides everything
  // Otherwise combine context path with segments
  const namespace = useMemoPath(
    partial?.namespace ?? [...contextNamespace, ...segmentPath],
  )
  const stateAtom =
    partial?.stateAtom ?? context?.stateAtom ?? atom<StateType>({})
  const stateAtomAccessor =
    partial?.stateAtomAccessor ?? context?.stateAtomAccessor

  return useMemo(
    () => ({
      namespace,
      stateAtom,
      stateAtomAccessor,
    }),
    [namespace, stateAtom, stateAtomAccessor],
  )
}

/**
 * Hook to get the parent state namespace from context
 * @returns {StateNamespace} The parent state namespace or an empty StateNamespace
 */
export function useParentStateNamespace(): StateNamespace {
  const context = useContext(StateNamespaceContext)
  return useMemo(
    () => context ?? { namespace: [], stateAtom: atom<StateType>({}) },
    [context],
  )
}

export function useStateAtom<T>(
  namespace: StateNamespace | null,
  key: string,
  defaultValue: T,
): [T, (update: T | ((prev: T) => T)) => void] {
  const context = useParentStateNamespace()
  const path = namespace?.namespace ?? context.namespace
  const accessor = namespace?.stateAtomAccessor ?? context.stateAtomAccessor
  const hasAccessor = !!accessor

  // Backend accessor mode: each atom is a separate StateAtom resource.
  // Always called (hooks must be unconditional), uses nullAccessor when inactive.
  const storeId = useMemo(() => [...path, key].join('/'), [path, key])
  const backendResult = useBackendStateAtom<T>(
    accessor ?? nullAccessor,
    storeId,
    defaultValue,
  )

  // Legacy in-memory atom mode: nested object tree.
  const stateAtom = namespace?.stateAtom ?? context.stateAtom
  const getValue = useCallback(() => {
    const state = stateAtom.get()
    return getDeepValue(state, path, key, defaultValue)
  }, [stateAtom, defaultValue, key, path])

  const defaultValueLatest = useLatestRef(defaultValue)
  const setValue = useCallback(
    (update: T | ((prev: T) => T)) => {
      if (!stateAtom) return
      const newValue =
        typeof update === 'function' ?
          (update as (prev: T) => T)(getValue())
        : update
      const state = stateAtom.get()
      stateAtom.set(
        setDeepValue(state, path, key, newValue, defaultValueLatest.current),
      )
    },
    [stateAtom, path, key, getValue, defaultValueLatest],
  )

  const legacyValue = useSyncExternalStore(
    (callback) => stateAtom.subscribe(callback),
    getValue,
    getValue,
  )

  useRegisterStateAtomForDevTools(
    storeId,
    legacyValue,
    hasAccessor ? null : getStateAtomScope(stateAtom),
  )

  if (hasAccessor) return backendResult
  return [legacyValue, setValue]
}

function getDeepValue<T>(
  obj: StateType,
  path: string[],
  key: string,
  defaultValue: T,
): T {
  let current = obj
  for (const segment of path) {
    const next = current[segment]
    if (next == null || typeof next !== 'object') return defaultValue
    current = next as StateType
  }
  return key in current ? (current[key] as T) : defaultValue
}

export function atomWithLocalStorage<T>(key: string, initialValue: T): Atom<T> {
  if (typeof localStorage === 'undefined') {
    return new BasicAtom(initialValue)
  }
  return new StorageAtom(new LocalStorageImpl(), key, initialValue)
}

export function derivedAtom<T, U>(
  source: Atom<T>,
  deriveFn: (value: T) => U,
): Atom<U> {
  return new DerivedAtom(source, deriveFn)
}

export function atom<T>(value: T): Atom<T> {
  return new BasicAtom(value)
}

export function useStateReducerAtom<State, Action>(
  namespace: StateNamespace | null,
  key: string,
  reducer: (state: State, action: Action) => State,
  initialState: State,
): [State, (action: Action) => void] {
  const context = useParentStateNamespace()
  const accessor = namespace?.stateAtomAccessor ?? context.stateAtomAccessor
  const hasAccessor = !!accessor
  const path = useMemo(
    () => namespace?.namespace ?? context.namespace ?? [],
    [namespace?.namespace, context.namespace],
  )

  // Backend accessor mode: use useBackendStateAtom + local dispatch.
  const storeId = useMemo(() => [...path, key].join('/'), [path, key])
  const [backendState, setBackendState] = useBackendStateAtom<State>(
    accessor ?? nullAccessor,
    storeId,
    initialState,
  )
  const backendDispatch = useCallback(
    (action: Action) => {
      setBackendState((prev) => reducer(prev, action))
    },
    [setBackendState, reducer],
  )

  // Legacy in-memory atom mode.
  const parentStateAtom = context.stateAtom
  const getValue = useCallback(() => {
    if (!parentStateAtom) return initialState
    const state = parentStateAtom.get()
    return getDeepValue(state, path, key, initialState)
  }, [parentStateAtom, path, key, initialState])

  const dispatch = useCallback(
    (action: Action) => {
      const currentState = getValue()
      const newState = reducer(currentState, action)
      const state = parentStateAtom.get()
      parentStateAtom.set(
        setDeepValue(state, path, key, newState, initialState),
      )
    },
    [parentStateAtom, path, key, getValue, reducer, initialState],
  )

  const legacyState = useSyncExternalStore(
    (callback) => parentStateAtom.subscribe(callback) ?? (() => {}),
    getValue,
    getValue,
  )

  useRegisterStateAtomForDevTools(
    storeId,
    legacyState,
    hasAccessor ? null : getStateAtomScope(parentStateAtom),
  )

  if (hasAccessor) return [backendState, backendDispatch]
  return [legacyState, dispatch]
}

type RegisteredStateAtom = InspectableStateAtom & {
  setCurrent: (value: unknown) => void
}

function createRegisteredStateAtom(): RegisteredStateAtom {
  let current: unknown = {}
  const listeners = new Set<() => void>()

  return {
    get() {
      return current
    },
    subscribe(callback: () => void) {
      listeners.add(callback)
      return () => listeners.delete(callback)
    },
    setCurrent(value: unknown) {
      if (isDeepEqual(current, value)) return
      current = value
      listeners.forEach((listener) => listener())
    },
  }
}

function useRegisterStateAtomForDevTools(
  storeId: string,
  value: unknown,
  scope: StateAtomScope | null,
) {
  const shouldRegister = scope !== null && !storeId.startsWith('devtools/')
  const registry = useStateAtomRegistryContext()
  const registeredAtomRef = useRef<RegisteredStateAtom | null>(null)
  if (!registeredAtomRef.current) {
    registeredAtomRef.current = createRegisteredStateAtom()
  }

  useEffect(() => {
    if (!shouldRegister) return
    registeredAtomRef.current?.setCurrent(value)
  }, [shouldRegister, value])

  useEffect(() => {
    const registeredAtom = registeredAtomRef.current
    if (!shouldRegister || !registry || !registeredAtom || !scope) return
    registry.registerAtom(storeId, storeId, scope, registeredAtom)
    return () => {
      registry.unregisterAtom(storeId)
    }
  }, [registry, scope, shouldRegister, storeId])
}

function getStateAtomScope(atom: Atom<StateType>): StateAtomScope {
  return atom instanceof StorageAtom ? 'persistent' : 'local'
}

// Helper function to check if an object is empty
function isEmptyObject(obj: unknown): boolean {
  return (
    obj !== null && typeof obj === 'object' && Object.keys(obj).length === 0
  )
}

// Helper function to set deeply nested value
function setDeepValue(
  obj: StateType,
  keys: string[],
  key: string,
  value: unknown,
  defaultValue: unknown,
): StateType {
  if (keys.length === 0) {
    // If value equals default, remove the key
    if (isDeepEqual(value, defaultValue)) {
      const { [key]: _, ...rest } = obj
      return rest
    }
    return { ...obj, [key]: value }
  }

  const [first, ...rest] = keys
  const newSubTree = setDeepValue(
    (obj[first] as StateType) || {},
    rest,
    key,
    value,
    defaultValue,
  )

  // If the subtree is empty after modification, remove it entirely
  if (isEmptyObject(newSubTree)) {
    const { [first]: _, ...remaining } = obj
    return remaining
  }

  return {
    ...obj,
    [first]: newSubTree,
  }
}

/**
 * Component for debugging namespace state
 * Displays the current state for the active namespace
 */
export function StateDebugger() {
  const { namespace = [], stateAtom = null } =
    useContext(StateNamespaceContext) ?? {}
  const state = useSyncExternalStore(
    (callback) => stateAtom?.subscribe(callback) ?? (() => {}),
    () => stateAtom?.get() ?? {},
    () => stateAtom?.get() ?? {},
  )

  const getCurrentState = useMemo(() => {
    if (!stateAtom) return {}
    let current = state
    for (const segment of namespace) {
      current = (current[segment] as StateType) || {}
    }
    return current
  }, [state, namespace, stateAtom])

  const currentState = getCurrentState
  return <pre>{JSON.stringify(currentState, null, 2)}</pre>
}

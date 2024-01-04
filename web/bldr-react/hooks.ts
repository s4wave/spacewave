import {
  DependencyList,
  RefObject,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import isDeepEqual from 'lodash.isequal'
import { Client } from 'starpc'
import { useBldrContext } from './bldr-context.js'

import {
  WebDocument as BldrWebDocument,
  WebView as BldrWebView,
  RetryOpts,
  retryWithAbort,
} from '@aptre/bldr'
import { ValueCallback } from './callback.js'

// Destructor is the destructor type from React.
export type Destructor = () => void

// WebViewHostClientEffect is the callback function type for useWebViewHostClientImpl.
export type WebViewHostClientEffect = (
  client: Client,
  abortSignal: AbortSignal,
  webDocument: BldrWebDocument,
  webView: BldrWebView,
) => void | Destructor

// useWebViewHostClient builds a client and abort signal for the web view host.
export function useWebViewHostClient(
  effect: WebViewHostClientEffect,
  deps?: DependencyList,
) {
  const bldrContext = useBldrContext()
  const webDocument = bldrContext?.webDocument
  const webView = bldrContext?.webView
  useEffect(
    () => {
      if (!webDocument || !webView) {
        return
      }
      const client = webDocument.buildWebViewHostClient(webView.getUuid())
      const cancel = new AbortController()
      const destructor = effect(client, cancel.signal, webDocument, webView)
      return () => {
        cancel.abort()
        if (destructor) {
          destructor()
        }
      }
    },
    [webDocument, webView, ...(deps ?? [])], // eslint-disable-line
  )
}

// WebViewHostClientImplEffect is the callback function type for useWebViewHostClientImpl.
export type WebViewHostClientImplEffect<T> = (
  impl: T,
  abortSignal: AbortSignal,
  webDocument: BldrWebDocument,
  webView: BldrWebView,
  client: Client,
) => void | Destructor

// useWebViewHostClientImpl builds a client implementation and abort signal for the web view host.
export function useWebViewHostClientImpl<T>(
  ctor: (c: Client) => T,
  effect: WebViewHostClientImplEffect<T>,
  deps?: DependencyList,
) {
  useWebViewHostClient((client, abortSignal, webDocument, webView) => {
    return effect(ctor(client), abortSignal, webDocument, webView, client)
  }, deps)
}

// createWebViewHostClientImplEffect creates a useEffect function which calls useWebViewHostClientImpl.
export function createWebViewHostClientImplEffect<T>(
  ctor: (c: Client) => T,
): (effect: WebViewHostClientImplEffect<T>, deps?: DependencyList) => void {
  return (effect: WebViewHostClientImplEffect<T>, deps?: DependencyList) => {
    useWebViewHostClientImpl<T>(ctor, effect, deps)
  }
}

// createWebViewHostClientImplState creates a useState function for calling a rpc service impl.
export function createWebViewHostClientImplState<T>(
  ctor: (c: Client) => T,
): (deps?: DependencyList) => T | undefined {
  return (deps?: DependencyList) => {
    const [impl, setImpl] = useState<T | undefined>(undefined)
    useWebViewHostClient((client) => {
      setImpl(ctor(client))
      return () => setImpl(undefined)
    }, deps)
    return impl
  }
}

// useAbortSignal returns an AbortSignal which is canceled when the deps change.
export function useAbortSignal(deps: DependencyList = []): AbortSignal {
  const abortController = useMemo(
    () => new AbortController(),
    [...deps], // eslint-disable-line
  )
  useEffect(() => {
    return () => abortController.abort()
  }, [abortController])
  return abortController.signal
}

// useAbortSignalEffect wraps an effect with an abort signal.
export function useAbortSignalEffect(
  effect: (signal: AbortSignal) => void | (() => void),
  deps?: DependencyList,
) {
  useEffect(
    () => {
      const controller = new AbortController()
      const signal = controller.signal
      const teardown = effect(signal)

      return () => {
        controller.abort()
        if (teardown) {
          teardown()
        }
      }
    },
    [...(deps ?? [])], // eslint-disable-line
  )
}

// useRetryWithAbort calls the function with an abort signal and retries on error.
//
// will be aborted when the component is unmounted or deps change.
export function useRetryWithAbort(
  cb: (abortSignal: AbortSignal) => Promise<void>,
  opts?: RetryOpts,
  deps?: DependencyList,
) {
  useAbortSignalEffect((signal) => {
    retryWithAbort(signal, cb, opts)
  }, deps)
}

// useLatestRef returns a ref that contains the latest version of the value.
//
// changed is called when the value is changed from the initial value.
export function useLatestRef<T>(
  value: T,
  changed?: (value: T) => void,
): RefObject<T> {
  const ref = useRef(value)
  const changedRef = useRef(changed)
  useEffect(() => {
    changedRef.current = changed
  }, [changed])
  useEffect(() => {
    if (ref.current !== value) {
      ref.current = value
      if (changedRef.current) {
        changedRef.current(value)
      }
    }
  }, [value])
  return ref
}

// useMemoUint8Array memoizes a uint8array.
export function useMemoUint8Array(value: Uint8Array | null): Uint8Array | null {
  return useMemoDeepEqual(value)
}

// MouseEvent and other events satisfy this.
interface DetailCountEvent {
  // detail is the number of times the event occurred.
  detail: number
}

// useDetailCountHandler builds an event handler which correctly resets the
// event.detail counter when the component is re-mounted.
//
// The onClick event.detail contains the number of clicks: double-click has
// event.detail = 2. When the clicked React component is replaced, the
// event.detail does not reset.
//
// Question: https://stackoverflow.com/q/77719428/431369
// Issue: https://codesandbox.io/p/sandbox/react-on-click-event-detail-6ndl5v?file=%2Fsrc%2FApp.tsx%3A8%2C23
// Fix: https://codesandbox.io/p/sandbox/react-on-click-event-detail-possible-fix-4zwk7d?file=%2Fsrc%2FApp.tsx%3A59%2C1
export function useDetailCountHandler<E extends DetailCountEvent>(
  cb: (e: E, count: number) => void,
) {
  const stateRef = useRef({ prev: 0, sub: 0 })
  return useCallback(
    (e: E) => {
      const state = stateRef.current
      let count = e.detail
      if (state.sub >= count) {
        state.sub = 0
      }
      if (state.prev < count - 1) {
        state.sub = count - state.prev - 1
      }
      count -= state.sub
      state.prev = count
      cb(e, count)
    },
    [stateRef, cb],
  )
}

// GetStateFunc should return the latest state object.
//
// This should be a useCallback function with the deps of the values that are
// used within the update func.
//
// Returning undefined skips updating the state.
// Returning a value identical to the previous state skips emitting an update event.
export type GetStateFunc<T> = () => T | undefined

// useItState builds an AsyncIterable which emits the most recent state.
//
// When an iterator attaches to the AsyncIterable, the snapshot function is
// called to generate an initial message to send with the starting state.
//
// The getState function is called every time it changes (the parameter is
// updated). This function should be a useCallback with the dependency list set
// to the properties or state values used to build the state object. If it
// returns undefined or a value identical to the current state, the value will
// be skipped (do nothing). Otherwise the new value is emitted to any consumers.
//
// States are checked for deep equality and identical states are skipped.
//
// If skipSnapshot is set, the initial state value will be skipped.
// If latestValueOnly is set, slow consumers get the most recent state update only.
export function useItState<T>(
  getState: GetStateFunc<T>,
  skipSnapshot?: boolean,
  latestValueOnly?: boolean,
): AsyncIterable<T> {
  const latestValueOnlyRef = useLatestRef(latestValueOnly ?? false)
  const skipSnapshotRef = useLatestRef(skipSnapshot ?? false)

  const [state, setState] = useState<T | undefined>(undefined)

  const update = useCallback((getNextState: GetStateFunc<T>) => {
    setState((prev) => {
      const next = getNextState()
      if (
        typeof next === 'undefined' ||
        next === prev ||
        isDeepEqual(next, prev)
      ) {
        return prev
      }
      return next
    })
  }, [])

  useLatestRef(getState, update)

  const consumersRef = useRef<Set<ValueCallback<T>>>(new Set([]))
  const lastState = useRef<T | undefined>(undefined)
  useEffect(() => {
    if (lastState.current !== state) {
      lastState.current = state

      if (typeof state !== 'undefined') {
        const consumers = consumersRef.current
        for (const consumer of consumers.values()) {
          consumer(state)
        }
      }
    }
  }, [state])

  return useMemo(
    () => ({
      [Symbol.asyncIterator]: async function* () {
        if (
          !skipSnapshotRef.current &&
          typeof lastState.current !== 'undefined'
        ) {
          yield lastState.current
        }

        // Keep a send queue of changes to write.
        const sendQueue: T[] = []

        // Wake wakes the send loop.
        let wakeResolve: (() => void) | null = null

        // Register the consumer.
        const consumer: ValueCallback<T> = (value) => {
          if (latestValueOnlyRef.current) {
            sendQueue.length = 0
          }
          sendQueue.push(value)
          if (wakeResolve) {
            wakeResolve()
            wakeResolve = null
          }
        }

        consumersRef.current.add(consumer)

        try {
          while (true) {
            const waitWake = new Promise<void>((resolve) => {
              wakeResolve = resolve
            })

            const tx = sendQueue.splice(0)
            for (const out of tx) {
              yield out
            }

            await waitWake
          }
        } finally {
          consumersRef.current.delete(consumer)
        }
      },
    }),
    [latestValueOnlyRef, skipSnapshotRef],
  )
}

// GetUpdateFunc should return a message to send as an update to the prev state.
///
// This should be a useCallback function with the deps set to values that are
// used within the update func.
//
// Returning undefined skips emitting a state update.
export type GetUpdateFunc<T> = () => T | undefined

// GetSnapshotFunc is a function returning an initial snapshot message emitted
// when a consumer attaches to the iterable.
//
// If the function returns undefined, the initial snapshot message is skipped.
export type GetSnapshotFunc<T> = () => T | undefined

// useItUpdate builds an AsyncIterable which emits an initial snapshot message
// followed by update messages.
//
// When an iterator attaches to the AsyncIterable, the snapshot function is
// called to generate an initial message to send with the starting state.
//
// The getUpdateUpdate function is called every time it changes (the parameter
// is updated). This function should be a useCallback with the dependency list
// set to the properties or state values used to build the state object. If it
// returns undefined, the value will be skipped (do nothing). Otherwise the new
// value will be emitted to the listeners of the AsyncIterable.
//
// If latestValueOnly is set, slow consumers get the most recent state update only.
// If any of the deps change the AsyncIterable object will be re-created.
export function useItUpdate<T>(
  getSnapshot: GetSnapshotFunc<T>,
  getUpdate: GetUpdateFunc<T>,
  latestValueOnly?: boolean,
  deps?: DependencyList,
) {
  const getSnapshotRef = useLatestRef(getSnapshot)
  const latestValueOnlyRef = useLatestRef(latestValueOnly ?? false)

  const consumersRef = useRef<Set<ValueCallback<T>>>(new Set([]))
  useLatestRef(getUpdate, (nextGetUpdate) => {
    const next = nextGetUpdate()
    if (typeof next !== 'undefined') {
      for (const consumer of consumersRef.current.values()) {
        consumer(next)
      }
    }
  })

  return useMemo(
    () => ({
      [Symbol.asyncIterator]: async function* () {
        if (getSnapshotRef.current) {
          const snapshot = getSnapshotRef.current()
          if (typeof snapshot !== 'undefined') {
            yield snapshot
          }
        }

        // Keep a send queue of changes to write.
        const sendQueue: T[] = []

        // Wake wakes the send loop.
        let wakeResolve: (() => void) | null = null

        // Register the consumer.
        const consumer: ValueCallback<T> = (value) => {
          if (latestValueOnlyRef.current) {
            sendQueue.length = 0
          }
          sendQueue.push(value)
          if (wakeResolve) {
            wakeResolve()
            wakeResolve = null
          }
        }

        consumersRef.current.add(consumer)

        try {
          while (true) {
            const waitWake = new Promise<void>((resolve) => {
              wakeResolve = resolve
            })

            const tx = sendQueue.splice(0)
            for (const out of tx) {
              yield out
            }

            await waitWake
          }
        } finally {
          consumersRef.current.delete(consumer)
        }
      },
    }),
    [...(deps ?? [])], // eslint-disable-line
  )
}

// useMemoDeepEqual checks if the given value is deep equal to the memoized value
// and returns the memoized value if so.
export function useMemoDeepEqual<T>(
  value: T,
  checkEqual: (v1: T, v2: T) => boolean = isDeepEqual,
): T {
  const [memoValue, setMemoValue] = useState<T>(() => value)
  const memoEquiv = useMemo(
    () => value === memoValue || checkEqual(value, memoValue),
    [memoValue, value, checkEqual],
  )
  useEffect(() => {
    if (!memoEquiv) {
      setMemoValue(value)
    }
  }, [memoEquiv, value])
  return memoEquiv ? memoValue : value
}

// useMemoDeepEqualGetter checks if the given value is deep equal to the
// memoized value and returns the memoized value if so. If the value is
// different, calls the getter to return the next value.
export function useMemoDeepEqualGetter<T, V = T>(
  value: T,
  getter: (val: T) => V,
  checkEqual: (v1: T, v2: T) => boolean = isDeepEqual,
): V {
  const [memoState, setMemoState] = useState<{
    memoValue: T
    outValue: V
  }>(() => ({ memoValue: value, outValue: getter(value) }))
  const memoValue = memoState.memoValue
  const memoEquiv = useMemo(
    () => value === memoValue || checkEqual(value, memoValue),
    [value, memoValue, checkEqual],
  )
  const outValue = memoEquiv ? memoState.outValue : getter(value)
  useEffect(() => {
    if (!memoEquiv) {
      setMemoState({ memoValue: value, outValue: outValue })
    }
  }, [memoEquiv, value, outValue])
  return outValue
}

// setDeepEqual generates a setter which checks if the two values are deep-equal.
export function setDeepEqual<S>(next: S): (prevState: S | null) => S {
  return (prev: S | null): S => {
    if (!prev) return next
    return prev === next || isDeepEqual(prev, next) ? prev : next
  }
}

// useWatchStateRpc uses a RPC function which returns an updated value when the state changes.
// Returns the latest message returned by the RPC call.
// If the rpc function passed is null, returns null for the value.
// Restarts the RPC if the rpc function changes.
export function useWatchStateRpc<T>(
  watchStateRpc:
    | ((abortSignal: AbortSignal) => AsyncIterable<T>)
    | null
    | undefined,
  retryOpts?: RetryOpts,
  deps?: DependencyList,
): T | null {
  const [currValue, setCurrValue] = useState<T | null>(null)
  const handleValue = useCallback(
    (nextValue: T) => setCurrValue(setDeepEqual<T | null>(nextValue)),
    [],
  )

  useRetryWithAbort(
    async (signal) => {
      if (!watchStateRpc) {
        setCurrValue(null)
        return
      }
      const stream = watchStateRpc(signal)
      for await (const resp of stream) {
        handleValue(resp)
      }
    },
    retryOpts,
    [watchStateRpc, ...(deps ?? [])],
  )

  return currValue === null || !watchStateRpc ? null : currValue
}

// useSetValueRpc uses a RPC function which sets the given value via an rpc when it changes.
// If the value, setValueRpc, or deps change the function will be called again.
// Returns if the state has been set successfully yet.
export function useSetValueRpc<T>(
  value: T,
  setValueRpc:
    | ((value: T, abortSignal: AbortSignal) => Promise<void>)
    | null
    | undefined,
  retryOpts?: RetryOpts,
  deps?: DependencyList,
): boolean {
  const currValue = useMemoDeepEqual(value)
  const [wasSet, setWasSet] = useState(false)

  useRetryWithAbort(
    async (signal) => {
      if (!setValueRpc) {
        setWasSet(false)
        return
      }
      await setValueRpc(currValue, signal)
      setWasSet(true)
    },
    retryOpts,
    [setValueRpc, currValue, ...(deps ?? [])],
  )

  return wasSet
}

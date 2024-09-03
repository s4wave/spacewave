import {
  DependencyList,
  RefObject,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
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

// WebViewHostClientEffect is the callback function type for useWebViewHostClient.
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

// WebViewHostServiceClientEffect is the callback function type for useWebViewHostServiceClient.
export type WebViewHostServiceClientEffect<T> = (
  impl: T,
  abortSignal: AbortSignal,
  webDocument: BldrWebDocument,
  webView: BldrWebView,
  client: Client,
) => void | Destructor

// useWebViewHostServiceClient builds a client implementation and abort signal for the web view host.
export function useWebViewHostServiceClient<T>(
  ctor: (c: Client) => T,
  effect: WebViewHostServiceClientEffect<T>,
  deps?: DependencyList,
) {
  useWebViewHostClient((client, abortSignal, webDocument, webView) => {
    return effect(ctor(client), abortSignal, webDocument, webView, client)
  }, deps)
}

// createWebViewHostClientEffect creates a useEffect function which calls useWebViewHostClient.
export function createWebViewHostClientEffect<T>(
  ctor: (c: Client) => T,
): (effect: WebViewHostServiceClientEffect<T>, deps?: DependencyList) => void {
  return (effect: WebViewHostServiceClientEffect<T>, deps?: DependencyList) => {
    useWebViewHostServiceClient<T>(ctor, effect, deps)
  }
}

// createWebViewHostClientState creates a useState function for calling a rpc service impl.
export function createWebViewHostClientState<T>(
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
  return useMemoEqual(value)
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

// EqualStateFunc should compare two states for equality.
export type EqualStateFunc<T> = (t1: T, t2: T) => boolean

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
  cmpState?: EqualStateFunc<T>,
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
        (cmpState && typeof prev !== 'undefined' && cmpState(prev, next))
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

// useMemoEqual checks if the given value is equal to the memoized value
// and returns the memoized value if so.
export function useMemoEqual<T>(
  value: T,
  checkEqual?: (v1: NonNullable<T>, v2: NonNullable<T>) => boolean,
): T {
  const [memoValue, setMemoValue] = useState<T>(() => value)
  const memoEquiv = useMemo(
    () =>
      value === memoValue ||
      (value == null) === (memoValue == null) ||
      (value != null &&
        memoValue != null &&
        checkEqual &&
        checkEqual(value, memoValue)),
    [memoValue, value, checkEqual],
  )
  useEffect(() => {
    if (!memoEquiv) {
      setMemoValue(value)
    }
  }, [memoEquiv, value])
  return memoEquiv ? memoValue : value
}

// useMemoEqualGetter checks if the given value is equal to the memoized value
// and returns the memoized value if so. If the value is different, calls the
// getter to return the next value.
export function useMemoEqualGetter<T, V = T>(
  value: T,
  getter: (val: T) => V,
  checkEqual: (v1: NonNullable<T>, v2: NonNullable<T>) => boolean,
): V {
  const [memoState, setMemoState] = useState<{
    memoValue: T
    outValue: V
  }>(() => ({ memoValue: value, outValue: getter(value) }))
  const memoValue = memoState.memoValue
  const memoEquiv = useMemo(
    () =>
      value === memoValue ||
      (value == null) === (memoValue == null) ||
      (value != null && memoValue != null && checkEqual(value, memoValue)),
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

// setIfChanged generates a setter which checks if the two values are equal and preserves the old value if not.
export function setIfChanged<S>(
  next: S,
  checkEqual?: (v1: NonNullable<S>, v2: NonNullable<S>) => boolean,
): (prevState: S | null) => S {
  return (prev: S | null): S => {
    if (prev == null || next == null) return next
    return prev === next || (checkEqual && checkEqual(prev, next)) ? prev : next
  }
}

// useWatchStateRpc uses a RPC function which returns an updated value when the state changes.
// Returns the latest message returned by the RPC call.
// If the rpc function or request message passed is null, returns null for the value.
// Restarts the RPC if the rpc function or the request argument changes.
// checkEqual checks if two response objects are equal.
//
// T is the response type.
// R is the request type.
export function useWatchStateRpc<T, R = {}>(
  watchStateRpc:
    | ((req: R, abortSignal: AbortSignal) => AsyncIterable<T>)
    | ((req: R, abortSignal: AbortSignal) => AsyncIterable<T> | null)
    | null
    | undefined,
  req: R | null | undefined,
  checkReqEqual: (v1: R, v2: R) => boolean,
  checkRespEqual?: (v1: T, v2: T) => boolean,
  retryOpts?: RetryOpts,
  deps?: DependencyList,
): T | null {
  const [currValue, setCurrValue] = useState<T | null>(null)
  const handleValue = useCallback(
    (nextValue: T) =>
      setCurrValue(setIfChanged<T | null>(nextValue, checkRespEqual)),
    [],
  )
  const memoizedReq = useMemoEqual(req, checkReqEqual)

  useRetryWithAbort(
    async (signal) => {
      if (watchStateRpc == null || memoizedReq == null || signal.aborted) {
        setCurrValue(null)
        return
      }

      const stream = watchStateRpc(memoizedReq, signal)
      if (!stream) {
        setCurrValue(null)
        return
      }

      for await (const resp of stream) {
        handleValue(resp)
      }
    },
    retryOpts,
    [watchStateRpc, memoizedReq, ...(deps ?? [])],
  )

  return currValue == null || watchStateRpc == null || memoizedReq == null ?
      null
    : currValue
}

// useSetValueRpc uses a RPC function which sets the given value via an rpc when it changes.
// If the value, setValueRpc, or deps change the function will be called again.
// Returns if the state has been set successfully yet.
// If the value is null or undefined: does nothing.
export function useSetValueRpc<T>(
  value: T | null | undefined,
  setValueRpc:
    | ((value: T, abortSignal: AbortSignal) => Promise<void>)
    | null
    | undefined,
  retryOpts?: RetryOpts,
  deps?: DependencyList,
  checkEqual?: (v1: NonNullable<T>, v2: NonNullable<T>) => boolean,
): boolean {
  const currValue = useMemoEqual<T | null | undefined>(value, checkEqual)
  const [wasSet, setWasSet] = useState(false)

  useRetryWithAbort(
    async (signal) => {
      if (!setValueRpc || currValue == null) {
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

/**
 * Executes a callback when the monitored value changes from a non-target value to the target value, excluding the first render.
 * The callback can optionally return a boolean to prevent the update of the monitored value.
 *
 * @param value - The value to be monitored.
 * @param targetValue - The value at which the callback should be executed.
 * @param callback - The function to execute when the value changes to the target value. Can return boolean to control update.
 * @param deps - Optional additional dependencies for the effect.
 */
export function useOnChangeToValue<T>(
  value: T,
  targetValue: T,
  callback: () => void | boolean,
  deps?: DependencyList,
): void {
  const [, setCurrValue] = useState<T>(() => value)

  useEffect(() => {
    setCurrValue((prev) => {
      if (prev !== value) {
        if (value === targetValue) {
          const result = callback()
          return (result ?? true) ? value : prev
        } else {
          return value
        }
      }
      return prev
    })
  }, [
    value,
    targetValue,
    callback,
    ...(deps || []), // eslint-disable-line
  ])
}

// Focusable-is-an-object-with-a-focus-functionh.
type Focusable = {
  focus: () => void
}

/**
 * Calls the focus method on the ref's current value when a specified value changes to a target value.
 * Ensures the ref's current value is truthy and has a focus method before attempting to focus.
 * @param ref - A React ref object potentially containing an element with a focus method.
 * @param value - The value to monitor for changes.
 * @param targetValue - The value at which the focus method should be called.
 */
export function useFocusOnValueChange<T extends Focusable, V>(
  ref: RefObject<T>,
  value: V,
  targetValue: V,
  deps?: DependencyList,
): void {
  const callback = useCallback(() => {
    if (ref.current) {
      ref.current.focus()
      return true // Return true to update previousValue.current
    }
    return false // Return false to prevent updating previousValue.current
  }, [ref])
  useOnChangeToValue(value, targetValue, callback, deps)
}

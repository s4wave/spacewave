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
  compareUint8Arrays,
} from '@aptre/bldr'
import { ValueCallback } from './callback.js'

/** Destructor is the destructor type from React. */
export type Destructor = () => void

/**
 * WebViewHostClientEffect is the callback function type for useWebViewHostClient.
 * @param client - The RPC client instance
 * @param abortSignal - Signal for aborting operations
 * @param webDocument - The web document instance
 * @param webView - The web view instance
 * @returns Optional destructor function
 */
export type WebViewHostClientEffect = (
  client: Client,
  abortSignal: AbortSignal,
  webDocument: BldrWebDocument,
  webView: BldrWebView,
) => void | Destructor

/**
 * Builds a client and abort signal for the web view host.
 * @param effect - The effect callback to run with the client
 * @param deps - Optional dependencies that trigger effect re-runs
 */
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

/**
 * Callback function type for useWebViewHostServiceClient.
 * @template T - The service client implementation type
 * @param impl - The service client implementation
 * @param abortSignal - Signal for aborting operations
 * @param webDocument - The web document instance
 * @param webView - The web view instance
 * @param client - The base RPC client instance
 * @returns Optional destructor function
 */
export type WebViewHostServiceClientEffect<T> = (
  impl: T,
  abortSignal: AbortSignal,
  webDocument: BldrWebDocument,
  webView: BldrWebView,
  client: Client,
) => void | Destructor

/**
 * Builds a client implementation and abort signal for the web view host.
 * @template T - The service client implementation type
 * @param ctor - Constructor function that creates the service client
 * @param effect - The effect callback to run with the client
 * @param deps - Optional dependencies that trigger effect re-runs
 */
export function useWebViewHostServiceClient<T>(
  ctor: (c: Client) => T,
  effect: WebViewHostServiceClientEffect<T>,
  deps?: DependencyList,
) {
  useWebViewHostClient((client, abortSignal, webDocument, webView) => {
    return effect(ctor(client), abortSignal, webDocument, webView, client)
  }, deps)
}

/**
 * Creates a useEffect function which calls useWebViewHostClient.
 * @template T - The service client implementation type
 * @param ctor - Constructor function that creates the service client
 * @returns A function that takes an effect callback and optional dependencies
 */
export function createWebViewHostClientEffect<T>(
  ctor: (c: Client) => T,
): (effect: WebViewHostServiceClientEffect<T>, deps?: DependencyList) => void {
  return (effect: WebViewHostServiceClientEffect<T>, deps?: DependencyList) => {
    useWebViewHostServiceClient<T>(ctor, effect, deps)
  }
}

/**
 * Creates a useState function for calling a RPC service implementation.
 * @template T - The service client implementation type
 * @param ctor - Constructor function that creates the service client
 * @returns A function that takes optional dependencies and returns the client implementation
 */
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

/**
 * Returns an AbortSignal which is canceled when the deps change.
 * @param deps - Optional dependencies that trigger signal abortion when changed
 * @returns An AbortSignal that is canceled on dependency changes
 */
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

/**
 * Wraps an effect with an abort signal.
 * @param effect - The effect callback that receives an abort signal
 * @param deps - Optional dependencies that trigger effect re-runs
 */
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

/**
 * Calls the function with an abort signal and retries on error.
 * Will be aborted when the component is unmounted or deps change.
 *
 * @param cb - The async callback function to retry
 * @param opts - Optional retry configuration options
 * @param deps - Optional dependencies that trigger retries when changed
 */
export function useRetryWithAbort(
  cb: (abortSignal: AbortSignal) => Promise<void>,
  opts?: RetryOpts,
  deps?: DependencyList,
) {
  useAbortSignalEffect((signal) => {
    retryWithAbort(signal, cb, opts)
  }, deps)
}

/**
 * Returns a ref that contains the latest version of the value.
 *
 * @param value - The value to track
 * @param changed - Optional callback called when the value changes from initial value
 * @returns A ref object containing the latest value
 */
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

/**
 * Memoizes a Uint8Array to prevent unnecessary re-renders.
 *
 * @param value - The Uint8Array to memoize
 * @returns The memoized Uint8Array
 */
export function useMemoUint8Array(value: Uint8Array | null): Uint8Array | null {
  return useMemoEqual(value, compareUint8Arrays)
}

/** Event interface for events that include a detail count */
interface DetailCountEvent {
  /** The number of times the event occurred */
  detail: number
}

/**
 * Builds an event handler which correctly resets the event.detail counter when the component is re-mounted.
 *
 * The onClick event.detail contains the number of clicks: double-click has event.detail = 2.
 * When the clicked React component is replaced, the event.detail does not reset.
 *
 * @see {@link https://stackoverflow.com/q/77719428/431369}
 * @see {@link https://codesandbox.io/p/sandbox/react-on-click-event-detail-6ndl5v}
 * @see {@link https://codesandbox.io/p/sandbox/react-on-click-event-detail-possible-fix-4zwk7d}
 *
 * @template E - The event type extending DetailCountEvent
 * @param cb - Callback function receiving the event and corrected count
 * @returns Event handler function
 */
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

/**
 * Function that returns the latest state object.
 *
 * Should be a useCallback function with deps of values used within the update func.
 * Returning undefined skips updating the state.
 * Returning a value identical to the previous state skips emitting an update event.
 *
 * @template T - The state type
 */
export type GetStateFunc<T> = () => T | undefined

/**
 * Function that compares two states for equality.
 *
 * @template T - The state type
 */
export type EqualStateFunc<T> = (t1: T, t2: T) => boolean

/**
 * Builds an AsyncIterable which emits the most recent state.
 *
 * When an iterator attaches to the AsyncIterable, the snapshot function is
 * called to generate an initial message to send with the starting state.
 *
 * The getState function is called every time it changes. This function should
 * be a useCallback with deps set to the properties or state values used to
 * build the state object. If it returns undefined or a value identical to
 * the current state, the value will be skipped. Otherwise the new value is
 * emitted to any consumers.
 *
 * @template T - The state type
 * @param getState - Function that returns the latest state
 * @param skipSnapshot - If true, skip sending initial state value
 * @param latestValueOnly - If true, slow consumers get only most recent state
 * @param cmpState - Optional function to compare states for equality
 * @returns AsyncIterable that emits state updates
 */
export function useItState<T>(
  getState: GetStateFunc<T>,
  skipSnapshot?: boolean,
  latestValueOnly?: boolean,
  cmpState?: EqualStateFunc<T>,
): AsyncIterable<T> {
  const latestValueOnlyRef = useLatestRef(latestValueOnly ?? false)
  const skipSnapshotRef = useLatestRef(skipSnapshot ?? false)

  const [state, setState] = useState<T | undefined>(undefined)

  const update = useCallback(
    (getNextState: GetStateFunc<T>) => {
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
    },
    [cmpState],
  )

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

/**
 * Function that returns a message to send as an update to the previous state.
 *
 * Should be a useCallback function with deps set to values used within the update func.
 * Returning undefined skips emitting a state update.
 *
 * @template T - The update message type
 */
export type GetUpdateFunc<T> = () => T | undefined

/**
 * Function returning an initial snapshot message emitted when a consumer attaches to the iterable.
 *
 * If the function returns undefined, the initial snapshot message is skipped.
 *
 * @template T - The snapshot message type
 */
export type GetSnapshotFunc<T> = () => T | undefined

/**
 * Builds an AsyncIterable which emits an initial snapshot message followed by update messages.
 *
 * When an iterator attaches to the AsyncIterable, the snapshot function is called
 * to generate an initial message to send with the starting state.
 *
 * The getUpdate function is called every time it changes. This function should be
 * a useCallback with deps set to the properties or state values used to build
 * the state object. If it returns undefined, the value will be skipped. Otherwise
 * the new value will be emitted to the listeners.
 *
 * @template T - The message type
 * @param getSnapshot - Function returning initial snapshot message
 * @param getUpdate - Function returning update messages
 * @param latestValueOnly - If true, slow consumers get only most recent update
 * @param deps - Dependencies that trigger AsyncIterable recreation
 * @returns AsyncIterable that emits snapshot and update messages
 */
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

/**
 * Memoizes a value using custom equality comparison.
 * Returns the memoized value if the new value is considered equal.
 *
 * @template T - The value type
 * @param value - The value to potentially memoize
 * @param checkEqual - Optional function to compare values for equality
 * @returns The memoized value if equal, otherwise the new value
 */
export function useMemoEqual<T>(
  value: T,
  checkEqual?: (v1: NonNullable<T>, v2: NonNullable<T>) => boolean,
): T {
  const ref = useRef<T>(value)

  const isEqual =
    value === ref.current ||
    (value != null &&
      ref.current != null &&
      checkEqual &&
      checkEqual(value as NonNullable<T>, ref.current as NonNullable<T>))

  if (!isEqual) {
    ref.current = value
  }

  return ref.current
}

/**
 * Memoizes a derived value using custom equality comparison on the input.
 * If the input value changes, calls the getter to compute the new derived value.
 *
 * @template T - The input value type
 * @template V - The derived value type
 * @param value - The input value
 * @param getter - Function to compute derived value from input
 * @param checkEqual - Function to compare input values for equality
 * @returns The memoized derived value
 */
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

/**
 * Uses a RPC function to watch for state changes and return updated values.
 *
 * Returns the latest message from the RPC call. Returns null if the RPC function
 * or request message is null. Restarts the RPC if the function or request changes.
 *
 * @template T - The response type
 * @template R - The request type
 * @param watchStateRpc - RPC function that returns state updates
 * @param req - The request message to send
 * @param checkReqEqual - Function to compare request objects
 * @param checkRespEqual - Optional function to compare response objects
 * @param retryOpts - Optional retry configuration
 * @param deps - Optional dependencies that trigger RPC restart
 * @returns The latest state value or null
 */
export function useWatchStateRpc<T, R = unknown>(
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
    [checkRespEqual],
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

/**
 * Uses a RPC function to get a value via RPC when the request changes.
 * Similar to useWatchStateRpc but for one-time RPC calls that return a single value.
 *
 * @template T - The response value type
 * @template R - The request type
 * @param getValueRpc - RPC function to call to get the value
 * @param req - The request value to send
 * @param checkReqEqual - Function to compare request values
 * @param checkRespEqual - Optional function to compare response values
 * @param retryOpts - Optional retry configuration
 * @param deps - Optional dependencies that trigger RPC restart
 * @returns The value returned by the RPC call, or null if unavailable
 */
export function useGetValueRpc<T, R = unknown>(
  getValueRpc:
    | ((req: R, abortSignal: AbortSignal) => Promise<T>)
    | null
    | undefined,
  req: R | null | undefined,
  checkReqEqual: (v1: R, v2: R) => boolean,
  checkRespEqual?: (v1: T, v2: T) => boolean,
  retryOpts?: RetryOpts,
  deps?: DependencyList,
): T | null {
  const [currValue, setCurrValue] = useState<T | null>(null)
  const memoizedReq = useMemoEqual(req, checkReqEqual)

  useRetryWithAbort(
    async (signal) => {
      if (getValueRpc == null || memoizedReq == null || signal.aborted) {
        setCurrValue(null)
        return
      }

      const resp = await getValueRpc(memoizedReq, signal)
      setCurrValue(setIfChanged<T | null>(resp, checkRespEqual))
    },
    retryOpts,
    [getValueRpc, memoizedReq, ...(deps ?? [])],
  )

  return currValue == null || getValueRpc == null || memoizedReq == null ?
      null
    : currValue
}

/**
 * Uses a RPC function to set a value via RPC when it changes.
 * Returns a boolean indicating if the state was successfully set.
 * The RPC is expected to return immediately.
 *
 * @template T - The value type to set
 * @template R - The RPC response type
 * @param setValueRpc - RPC function to call when setting the value
 * @param req - The request value to send
 * @param checkReqEqual - Optional function to compare request values
 * @param retryOpts - Optional retry configuration
 * @param deps - Optional dependencies that trigger RPC restart
 * @returns Boolean indicating if the state was successfully set
 */
export function useSetValueRpc<T, R = unknown>(
  setValueRpc:
    | ((req: T, abortSignal: AbortSignal) => Promise<R>)
    | null
    | undefined,
  req: T | null | undefined,
  checkReqEqual?: (v1: NonNullable<T>, v2: NonNullable<T>) => boolean,
  retryOpts?: RetryOpts,
  deps?: DependencyList,
): boolean {
  const currValue = useMemoEqual<T | null | undefined>(req, checkReqEqual)
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
 * Executes a callback when the monitored value changes from a non-target value to the target value.
 * The callback can return a boolean to prevent updating the monitored value.
 * Does not execute on first render.
 *
 * @template T - The value type being monitored
 * @param value - The value to monitor for changes
 * @param targetValue - The value that triggers the callback
 * @param callback - Function to execute on change. Can return boolean to control update
 * @param deps - Optional dependencies that trigger effect re-runs
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

/**
 * Represents an element that can receive focus via a focus() method.
 * Common examples include HTML elements like input, button, or div with tabIndex.
 * The type allows null values to handle cases where the element may not exist.
 */
type Focusable = {
  focus: () => void
} | null

/**
 * Calls focus() on a ref's current value when a monitored value changes to a target value.
 * Only focuses if the ref's current value exists and has a focus method.
 *
 * @template T - Type extending Focusable interface
 * @template V - The value type being monitored
 * @param ref - React ref containing focusable element
 * @param value - The value to monitor for changes
 * @param targetValue - The value that triggers focus
 * @param deps - Optional dependencies that trigger effect re-runs
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

/**
 * Hook that tracks the document's visibility state.
 *
 * Returns the current visibility state of the document which can be:
 * - 'visible': the page content may be at least partially visible
 * - 'hidden': the page content is not visible (minimized or in background)
 * - 'prerender': the page content is being prerendered and not visible
 *
 * Automatically updates when the visibility state changes.
 *
 * @returns The current {@link DocumentVisibilityState} of the document
 */
export function useDocumentVisibility(): DocumentVisibilityState {
  const [documentVisibility, setDocumentVisibility] =
    useState<DocumentVisibilityState>(document.visibilityState)

  useEffect(() => {
    const listener = () => setDocumentVisibility(document.visibilityState)
    document.addEventListener('visibilitychange', listener)
    return () => document.removeEventListener('visibilitychange', listener)
  }, [])

  return documentVisibility
}

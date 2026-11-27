import { useEffect, useState } from 'react'
import { castToError } from 'starpc'

/**
 * UsePromiseResult is the result object returned by the usePromise hook.
 *
 * @typeParam T: The type of data returned by the promise
 */
export interface UsePromiseResult<T> {
  /** data is the resolved promise data, or undefined if not yet resolved. */
  data: T | undefined
  /** loading indicates whether the promise is pending. */
  loading: boolean
  /** error is any error from executing the promise, or null if no error. */
  error: Error | null
}

/**
 * usePromise manages async promise execution with loading and error states.
 *
 * The hook automatically handles promise cancellation when the component
 * unmounts or when the callback changes. The hook provides loading and error
 * states for UI feedback.
 *
 * @typeParam T: The type of data returned by the promise
 * @param callback: Function that receives an AbortSignal and returns a promise
 * to execute. Wrap with useCallback to control when the promise re-executes.
 * Return undefined to skip fetching (useful for conditional fetches).
 * @returns Object containing data, loading state, and error state
 *
 * @example
 * ```tsx
 * const { data: sessionInfo, loading, error } = usePromise(
 *   useCallback((signal) => session.getSessionInfo(signal), [session])
 * )
 * ```
 */
export function usePromise<T>(
  callback: (signal: AbortSignal) => Promise<T> | undefined,
): UsePromiseResult<T> {
  const [state, setState] = useState<UsePromiseResult<T>>({
    data: undefined,
    loading: true,
    error: null,
  })

  useEffect(() => {
    const controller = new AbortController()
    const promise = callback(controller.signal)

    if (!promise) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- data-fetching hook must synchronously clear state when no promise is provided
      setState({ data: undefined, loading: false, error: null })
      return
    }

    setState({ data: undefined, loading: true, error: null })

    promise
      .then((result) => {
        if (!controller.signal.aborted) {
          setState({ data: result, loading: false, error: null })
        }
      })
      .catch((err) => {
        if (!controller.signal.aborted) {
          setState({
            data: undefined,
            loading: false,
            error: castToError(err),
          })
        }
      })

    return () => {
      controller.abort()
    }
  }, [callback])

  return state
}

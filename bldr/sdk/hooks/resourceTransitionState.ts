import { useCallback, useRef, useState } from 'react'

export type ResourceTransitionState<T> = {
  value: T | null
  loading: boolean
  error: Error | null
}

export type ResourceTransitionRun = {
  generation: number
  signal: AbortSignal
  abort: () => void
}

export function useResourceTransitionVersion(
  values: readonly unknown[],
): number {
  const previousValuesRef = useRef<readonly unknown[] | null>(null)
  const versionRef = useRef(0)
  if (
    previousValuesRef.current === null ||
    !(
      previousValuesRef.current.length === values.length &&
      previousValuesRef.current.every((value, index) => value === values[index])
    )
  ) {
    previousValuesRef.current = [...values]
    versionRef.current += 1
  }

  return versionRef.current
}

export function useResourceTransitionState<T>(initialLoading: boolean) {
  const [state, setState] = useState<ResourceTransitionState<T>>({
    value: null,
    loading: initialLoading,
    error: null,
  })
  const currentGenerationRef = useRef(0)
  const startedVersionRef = useRef(0)

  const isPending = useCallback(
    (version: number) => startedVersionRef.current !== version,
    [],
  )

  const isCurrent = useCallback(
    (run: ResourceTransitionRun) =>
      !run.signal.aborted && run.generation === currentGenerationRef.current,
    [],
  )

  const setLoading = useCallback(() => {
    setState((prev) =>
      prev.loading && prev.error === null
        ? prev
        : { value: prev.value, loading: true, error: null },
    )
  }, [])

  const resetForRetry = useCallback(() => {
    setState({ value: null, loading: true, error: null })
  }, [])

  const settleNull = useCallback((version: number, loading: boolean) => {
    startedVersionRef.current = version
    currentGenerationRef.current += 1
    setState((prev) =>
      prev.value === null && prev.loading === loading && prev.error === null
        ? prev
        : { value: null, loading, error: null },
    )
  }, [])

  const begin = useCallback(
    (version: number): ResourceTransitionRun => {
      const abortController = new AbortController()
      currentGenerationRef.current += 1
      startedVersionRef.current = version
      setLoading()
      return {
        generation: currentGenerationRef.current,
        signal: abortController.signal,
        abort: () => abortController.abort(),
      }
    },
    [setLoading],
  )

  const complete = useCallback(
    (run: ResourceTransitionRun, value: T | null): boolean => {
      if (!isCurrent(run)) {
        return false
      }
      setState({ value, loading: false, error: null })
      return true
    },
    [isCurrent],
  )

  const fail = useCallback(
    (run: ResourceTransitionRun, error: Error): boolean => {
      if (!isCurrent(run)) {
        return false
      }
      setState({ value: null, loading: false, error })
      return true
    },
    [isCurrent],
  )

  return {
    state,
    isPending,
    isCurrent,
    setLoading,
    resetForRetry,
    settleNull,
    begin,
    complete,
    fail,
  }
}

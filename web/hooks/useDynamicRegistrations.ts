import { useCallback, useMemo } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import type { Root } from '@s4wave/sdk/root'

/** useDynamicRegistrations retains mapped identities while registrations agree. */
export function useDynamicRegistrations<TReq, TResp, TReg, TOutput>(
  root: Root | null | undefined,
  createStream: (
    root: Root,
    req: TReq,
    signal: AbortSignal,
  ) => AsyncIterable<TResp>,
  emptyReq: TReq,
  reqEquals: (a: TReq, b: TReq) => boolean,
  respEquals: (a: TResp, b: TResp) => boolean,
  getRegistrations: (resp: TResp | null) => TReg[],
  mapper: (reg: TReg) => TOutput | null,
  registrationEquals: (a: TReg, b: TReg) => boolean = Object.is,
): TOutput[] {
  // The mounted root owns its registry subscription and its projection cache.
  const watchFn = useCallback(
    (_: TReq, signal: AbortSignal) => {
      if (!root) return null
      return createStream(root, _, signal)
    },
    [root, createStream],
  )

  const watchState = useWatchStateRpc(watchFn, emptyReq, reqEquals, respEquals)

  // A changed registry snapshot must not remount unchanged lazy components.
  // Retain only current rows, and discard the cache when its root changes.
  const project = useMemo(() => {
    if (!root) {
      return (_registrations: TReg[]): TOutput[] => []
    }
    let previous: { registration: TReg; value: TOutput | null }[] = []
    return (registrations: TReg[]): TOutput[] => {
      const next = registrations.map(
        (registration) =>
          previous.find((entry) =>
            registrationEquals(entry.registration, registration),
          ) ?? { registration, value: mapper(registration) },
      )
      previous = next
      return next.flatMap((entry) =>
        entry.value === null ? [] : [entry.value],
      )
    }
  }, [root, mapper, registrationEquals])

  // Clearing the parent immediately removes its public registrations.
  return useMemo(() => {
    return project(root ? getRegistrations(watchState) : [])
  }, [root, watchState, getRegistrations, project])
}

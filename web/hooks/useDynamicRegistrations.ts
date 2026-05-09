import { useCallback, useMemo } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import type { Root } from '@s4wave/sdk/root'

// useDynamicRegistrations subscribes to a watch RPC that returns dynamic
// registrations, maps them through a converter, and returns the output array.
// Encapsulates the shared useWatchStateRpc + React.lazy pattern used by
// both the viewer registry and config type registry.
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
): TOutput[] {
  const watchFn = useCallback(
    (_: TReq, signal: AbortSignal) => {
      if (!root) return null
      return createStream(root, _, signal)
    },
    [root, createStream],
  )

  const watchState = useWatchStateRpc(watchFn, emptyReq, reqEquals, respEquals)

  return useMemo(() => {
    const regs = getRegistrations(watchState)
    return regs.flatMap((r) => {
      const item = mapper(r)
      return item ? [item] : []
    })
  }, [watchState, getRegistrations, mapper])
}

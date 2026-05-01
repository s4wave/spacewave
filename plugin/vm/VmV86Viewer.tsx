import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { LuSettings, LuTv } from 'react-icons/lu'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'

import { cn } from '@s4wave/web/style/utils.js'
import {
  SetV86StateOp,
  VmState,
  VmV86,
  VmMount,
  type V86Config,
} from '@s4wave/sdk/vm/v86.pb.js'
import { keyToIRI, iriToKey } from '@s4wave/sdk/world/graph-utils.js'
import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { UnixFSTypeID } from '@s4wave/web/hooks/useUnixFSHandle.js'

import { v86SerialChannelName, type SerialFrame } from './serial-channel.js'
import { VmV86TypeID } from './sdk/vmv86.js'

export { VmV86TypeID }

const SET_V86_STATE_OP_ID = 'spacewave/vm/v86/set-state'

// V86 graph predicates for per-asset overrides. Mirrors
// sdk/vm/v86.go PredV86{Kernel,Rootfs,Bios,Wasm}Override. When set, a
// per-VM override takes precedence over the V86Image's v86image/* edge at
// mount-resolve time.
type OverrideSlot = 'kernel' | 'rootfs' | 'bios' | 'wasm'

const OVERRIDE_PREDICATE: Record<OverrideSlot, string> = {
  kernel: '<v86/kernel-override>',
  rootfs: '<v86/rootfs-override>',
  bios: '<v86/bios-override>',
  wasm: '<v86/wasm-override>',
}

const OVERRIDE_LABEL: Record<OverrideSlot, string> = {
  kernel: 'Kernel',
  rootfs: 'Rootfs',
  bios: 'BIOS',
  wasm: 'WASM',
}

const OVERRIDE_SLOTS: OverrideSlot[] = ['kernel', 'rootfs', 'bios', 'wasm']

// vmStateLabel returns a compact human label for a VmState enum value.
function vmStateLabel(state: VmState | undefined): string {
  switch (state) {
    case VmState.VmState_STARTING:
      return 'starting'
    case VmState.VmState_RUNNING:
      return 'running'
    case VmState.VmState_STOPPING:
      return 'stopping'
    case VmState.VmState_ERROR:
      return 'error'
    case VmState.VmState_STOPPED:
    default:
      return 'stopped'
  }
}

function vmStateBadgeClass(state: VmState | undefined): string {
  switch (state) {
    case VmState.VmState_RUNNING:
      return 'bg-emerald-500/10 text-emerald-500'
    case VmState.VmState_STARTING:
    case VmState.VmState_STOPPING:
      return 'bg-amber-500/10 text-amber-500'
    case VmState.VmState_ERROR:
      return 'bg-red-500/10 text-red-500'
    default:
      return 'bg-muted text-muted-foreground'
  }
}

// VmV86Viewer displays a V86 virtual machine: an xterm-backed serial console,
// Start/Stop controls bound to SetV86StateOp, a list of configured mounts,
// and a VM info bar (memory, uptime, state).
export default function VmV86Viewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)

  // One-shot read of the VmV86 block. Re-runs when the underlying worldState
  // resource rotates.
  const vmState = useResource(
    worldState,
    async (world, signal) => {
      if (!world || !objectKey) return null
      const objState = await world.getObject(objectKey, signal)
      if (!objState) return null
      using _ = objState
      using cursor = await objState.accessWorldState(undefined, signal)
      const blockResp = await cursor.getBlock({}, signal)
      if (!blockResp.found || !blockResp.data) return null
      return VmV86.fromBinary(blockResp.data)
    },
    [objectKey],
  )

  const vm = vmState.value
  const vmStateValue = vm?.state
  const cfg: V86Config | undefined = vm?.config
  const mounts: VmMount[] = cfg?.mounts ?? []
  const memoryMb = cfg?.memoryMb ?? 0
  const createdAtMs = useMemo(() => {
    const ts = vm?.createdAt
    return ts ? ts.getTime() : 0
  }, [vm?.createdAt])

  // Apply SetV86StateOp with the requested target state.
  const applyVmState = useCallback(
    async (target: VmState) => {
      const world = worldState.value
      if (!world || !objectKey) return
      const op = SetV86StateOp.create({
        objectKey,
        state: target,
        errorMessage: '',
      })
      const data = SetV86StateOp.toBinary(op)
      await world.applyWorldOp(SET_V86_STATE_OP_ID, data, '')
    },
    [worldState, objectKey],
  )

  // Settings panel toggle.
  const [showSettings, setShowSettings] = useState(false)
  const toggleSettings = useCallback(() => setShowSettings((v) => !v), [])

  // Read the four override edges for this VmV86 into a {slot: objectKey} map.
  // A missing edge resolves to "" meaning "use V86Image default".
  const overridesResource = useResource(
    worldState,
    async (world: IWorldState, signal: AbortSignal) => {
      if (!world || !objectKey) return null
      const subject = keyToIRI(objectKey)
      const result: Record<OverrideSlot, string> = {
        kernel: '',
        rootfs: '',
        bios: '',
        wasm: '',
      }
      for (const slot of OVERRIDE_SLOTS) {
        const resp = await world.lookupGraphQuads(
          subject,
          OVERRIDE_PREDICATE[slot],
          undefined,
          undefined,
          1,
          signal,
        )
        const quads = resp.quads ?? []
        if (quads.length > 0 && quads[0].obj) {
          result[slot] = iriToKey(quads[0].obj)
        }
      }
      return result
    },
    [objectKey],
  )
  const overrides = overridesResource.value

  // List UnixFS objects in the user Space. These are the candidates for
  // per-asset override pickers; "(use V86Image default)" is the empty option.
  const unixfsListResource = useResource(
    worldState,
    async (world: IWorldState, signal: AbortSignal) => {
      if (!world) return [] as string[]
      return listObjectsWithType(world, UnixFSTypeID, signal)
    },
    [],
  )
  const unixfsKeys = useMemo(
    () => unixfsListResource.value ?? [],
    [unixfsListResource.value],
  )

  // applyOverride writes (or clears) the override edge for one slot.
  const applyOverride = useCallback(
    async (slot: OverrideSlot, nextKey: string) => {
      const world = worldState.value
      if (!world || !objectKey) return
      const subject = keyToIRI(objectKey)
      const predicate = OVERRIDE_PREDICATE[slot]
      const prevKey = overrides?.[slot] ?? ''
      if (prevKey === nextKey) return
      if (prevKey) {
        await world.deleteGraphQuad(subject, predicate, keyToIRI(prevKey))
      }
      if (nextKey) {
        await world.setGraphQuad(subject, predicate, keyToIRI(nextKey))
      }
      overridesResource.retry()
    },
    [worldState, objectKey, overrides, overridesResource],
  )

  const isRunningLike =
    vmStateValue === VmState.VmState_RUNNING ||
    vmStateValue === VmState.VmState_STARTING

  const handleStart = useCallback(() => {
    void applyVmState(VmState.VmState_RUNNING)
  }, [applyVmState])

  const handleStop = useCallback(() => {
    void applyVmState(VmState.VmState_STOPPED)
  }, [applyVmState])

  // xterm terminal + BroadcastChannel serial bridge. Guest-emitted bytes
  // arrive as dir=out frames and are written to the terminal; user input is
  // posted as dir=in frames back to the backend, which feeds them into COM1.
  const terminalHostRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const host = terminalHostRef.current
    if (!host || !objectKey) return
    const term = new Terminal({
      convertEol: true,
      cursorBlink: true,
      fontSize: 13,
      theme: { background: '#000000', foreground: '#e5e7eb' },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host)
    try {
      fit.fit()
    } catch {
      // host may be briefly unsized during mount; ignore one-off fit errors
    }

    const channel = new BroadcastChannel(v86SerialChannelName(objectKey))
    channel.onmessage = (ev: MessageEvent<SerialFrame>) => {
      const frame = ev.data
      if (!frame || frame.dir !== 'out') return
      if (typeof frame.byte === 'number') {
        term.write(String.fromCharCode(frame.byte))
      }
    }

    const disposeInput = term.onData((chunk) => {
      if (!chunk) return
      const frame: SerialFrame = { dir: 'in', text: chunk }
      channel.postMessage(frame)
    })

    const handleResize = () => {
      try {
        fit.fit()
      } catch {
        /* resize before host has dimensions */
      }
    }
    window.addEventListener('resize', handleResize)

    return () => {
      window.removeEventListener('resize', handleResize)
      disposeInput.dispose()
      channel.close()
      term.dispose()
    }
  }, [objectKey])

  // Live-ticking uptime label while the VM is running.
  const [nowMs, setNowMs] = useState(() => Date.now())
  useEffect(() => {
    if (!isRunningLike) return
    const id = window.setInterval(() => setNowMs(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [isRunningLike])
  const uptimeLabel = useMemo(() => {
    if (!isRunningLike || !createdAtMs) return '-'
    const seconds = Math.max(0, Math.floor((nowMs - createdAtMs) / 1000))
    const h = Math.floor(seconds / 3600)
    const m = Math.floor((seconds % 3600) / 60)
    const s = seconds % 60
    if (h > 0) return `${h}h${m}m${s}s`
    if (m > 0) return `${m}m${s}s`
    return `${s}s`
  }, [isRunningLike, createdAtMs, nowMs])

  return (
    <div className="bg-background-primary flex h-full w-full flex-col overflow-hidden">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center justify-between border-b px-4">
        <div className="text-foreground flex items-center gap-2 text-sm font-semibold select-none">
          <LuTv className="h-4 w-4" />
          <span className="tracking-tight">V86</span>
          <span
            className={cn(
              'rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide',
              vmStateBadgeClass(vmStateValue),
            )}
          >
            {vmStateLabel(vmStateValue)}
          </span>
        </div>
        <div className="flex items-center gap-1 text-xs">
          <button
            type="button"
            onClick={handleStart}
            disabled={isRunningLike}
            className={cn(
              'rounded px-2 py-0.5 transition-colors',
              isRunningLike
                ? 'bg-muted/40 text-muted-foreground/60 cursor-not-allowed'
                : 'bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20',
            )}
          >
            Start
          </button>
          <button
            type="button"
            onClick={handleStop}
            disabled={!isRunningLike}
            className={cn(
              'rounded px-2 py-0.5 transition-colors',
              !isRunningLike
                ? 'bg-muted/40 text-muted-foreground/60 cursor-not-allowed'
                : 'bg-red-500/10 text-red-500 hover:bg-red-500/20',
            )}
          >
            Stop
          </button>
          <button
            type="button"
            onClick={toggleSettings}
            aria-pressed={showSettings}
            title="Asset overrides"
            className={cn(
              'rounded px-2 py-0.5 transition-colors',
              showSettings
                ? 'bg-primary/10 text-primary'
                : 'text-muted-foreground hover:bg-muted/40',
            )}
          >
            <LuSettings className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
      <div ref={terminalHostRef} className="min-h-0 flex-1 overflow-hidden bg-black" />
      <div className="border-foreground/8 flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 border-t px-4 py-1 text-[11px]">
        <span className="text-muted-foreground">
          mem <span className="text-foreground font-medium">{memoryMb || '-'}MB</span>
        </span>
        <span className="text-muted-foreground">
          uptime <span className="text-foreground font-medium">{uptimeLabel}</span>
        </span>
        <span className="text-muted-foreground">
          state <span className="text-foreground font-medium">{vmStateLabel(vmStateValue)}</span>
        </span>
      </div>
      {showSettings && (
        <div className="border-foreground/8 shrink-0 border-t px-4 py-2 text-[11px]">
          <div className="text-muted-foreground mb-1 uppercase tracking-wide">
            Asset Overrides
          </div>
          {!overrides && (
            <div className="text-muted-foreground">
              {overridesResource.loading ? 'loading...' : '-'}
            </div>
          )}
          {overrides && (
            <div className="flex flex-col gap-1">
              {OVERRIDE_SLOTS.map((slot) => {
                const current = overrides[slot]
                return (
                  <div
                    key={slot}
                    className="flex items-center justify-between gap-2"
                  >
                    <span className="text-foreground w-16 shrink-0">
                      {OVERRIDE_LABEL[slot]}
                    </span>
                    <select
                      value={current}
                      onChange={(e) => {
                        void applyOverride(slot, e.target.value)
                      }}
                      className="bg-muted/40 text-foreground min-w-0 flex-1 rounded px-1 py-0.5 font-mono text-[11px]"
                    >
                      <option value="">(use V86Image default)</option>
                      {current &&
                        !unixfsKeys.includes(current) && (
                          <option value={current}>{current}</option>
                        )}
                      {unixfsKeys.map((key) => (
                        <option key={key} value={key}>
                          {key}
                        </option>
                      ))}
                    </select>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      )}
      {mounts.length > 0 && (
        <div className="border-foreground/8 shrink-0 border-t px-4 py-1 text-[11px]">
          <div className="text-muted-foreground mb-0.5 uppercase tracking-wide">Mounts</div>
          <div className="flex flex-col gap-0.5">
            {mounts.map((m, i) => (
              <div
                key={`${m.path ?? ''}-${i}`}
                className="flex items-center justify-between font-mono"
              >
                <span className="text-foreground">{m.path || '(unset)'}</span>
                <span className="text-muted-foreground">
                  {m.objectKey ? m.objectKey.slice(0, 12) : '-'}
                  {m.writable ? ' rw' : ' ro'}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

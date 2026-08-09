import { useCallback, useMemo, useRef, useState, type ReactNode } from 'react'
import { isDesktop } from '@aptre/bldr'
import { useWatchStateRpc } from '@aptre/bldr-react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { LuTerminal, LuTriangleAlert } from 'react-icons/lu'

import type { Root } from '@s4wave/sdk/root'
import {
  RootResourceServiceClient,
  type RootResourceService,
} from '@s4wave/sdk/root/root_srpc.pb.js'
import {
  ListenerYieldPrompt,
  RuntimeHandoffState,
  WatchListenerYieldPromptsRequest,
  WatchListenerYieldPromptsResponse,
  WatchRuntimeHandoffRequest,
  WatchRuntimeHandoffResponse,
} from '@s4wave/sdk/root/root.pb.js'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { cn } from '@s4wave/web/style/utils.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { RuntimeHandoffProvider } from '@s4wave/app/listener/RuntimeHandoffContext.js'

// ListenerYieldNotifier watches for daemon-control takeover prompts
// emitted by the native resource listener's yield broker. When a
// prompt arrives it surfaces a modal so the user can allow or deny
// the remote runtime takeover. It wraps descendants with the
// RuntimeHandoffProvider so runtime-dependent components can disable
// actions while the runtime is handed off to a remote peer.
export function ListenerYieldNotifier({
  rootResource,
  children,
}: {
  rootResource: Resource<Root>
  children?: ReactNode
}) {
  if (!isDesktop) {
    return <>{children}</>
  }
  return (
    <ListenerYieldNotifierInner rootResource={rootResource}>
      {children}
    </ListenerYieldNotifierInner>
  )
}

function ListenerYieldNotifierInner({
  rootResource,
  children,
}: {
  rootResource: Resource<Root>
  children?: ReactNode
}) {
  const root = rootResource.value
  const service: RootResourceService | null = useMemo(() => {
    if (!root) return null
    return new RootResourceServiceClient(root.client)
  }, [root])

  const watchPrompts = useCallback(
    (_: WatchListenerYieldPromptsRequest, signal: AbortSignal) => {
      if (!service) return null
      return service.WatchListenerYieldPrompts({}, signal)
    },
    [service],
  )

  const promptsResp: WatchListenerYieldPromptsResponse | null =
    useWatchStateRpc(
      watchPrompts,
      {},
      WatchListenerYieldPromptsRequest.equals,
      WatchListenerYieldPromptsResponse.equals,
    )

  const prompts = promptsResp?.prompts ?? []
  const active: ListenerYieldPrompt | null =
    prompts.length > 0 ? prompts[0] : null

  const [pendingDecision, setPendingDecision] = useState<
    null | 'allow' | 'deny'
  >(null)
  const respondedIdRef = useRef<string | null>(null)

  const respond = useCallback(
    (promptId: string, allow: boolean) => {
      if (!service) return
      respondedIdRef.current = promptId
      setPendingDecision(allow ? 'allow' : 'deny')
      service
        .RespondToListenerYieldPrompt({ promptId, allow })
        .catch((err) => {
          toast.error('Takeover response failed', { description: String(err) })
        })
        .finally(() => setPendingDecision(null))
    },
    [service],
  )

  const onOpenChange = useCallback(
    (open: boolean) => {
      if (open) return
      if (!active) return
      const id = active.promptId ?? ''
      if (respondedIdRef.current === id) return
      // Closing the dialog is treated as deny.
      respond(id, false)
    },
    [active, respond],
  )

  const handoff = useHandoffState(service)

  const requesterName = active?.requesterName?.trim() || 'unknown process'
  const socketPath = active?.socketPath || ''

  return (
    <RuntimeHandoffProvider state={handoff}>
      {handoff?.active && (
        <RuntimeHandoffBanner handoff={handoff} service={service} />
      )}

      <Dialog open={active != null} onOpenChange={onOpenChange}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <LuTriangleAlert className="text-warning size-5 shrink-0" />
              Allow command-line takeover?
            </DialogTitle>
            <DialogDescription>
              A local process is asking to take over Spacewave's shared runtime
              socket. If you allow it, that process will act with your runtime
              authority until you reclaim it from this window. Only continue if
              you started this process yourself.
            </DialogDescription>
          </DialogHeader>

          <div className="border-foreground/10 bg-background/30 rounded-lg border p-4">
            <div className="flex items-start gap-3">
              <div className="border-foreground/10 bg-background/60 flex size-10 shrink-0 items-center justify-center rounded-full border">
                <LuTerminal className="text-foreground-alt size-5" />
              </div>
              <div className="min-w-0 flex-1 space-y-2">
                <div className="space-y-0.5">
                  <p className="text-foreground-alt/60 text-[0.65rem] font-medium tracking-wider uppercase select-none">
                    Requesting runtime
                  </p>
                  <p className="text-foreground truncate text-sm font-medium">
                    {requesterName}
                  </p>
                </div>
                <div className="space-y-0.5">
                  <p className="text-foreground-alt/60 text-[0.65rem] font-medium tracking-wider uppercase select-none">
                    Socket path
                  </p>
                  <p className="text-foreground-alt truncate font-mono text-xs">
                    {socketPath}
                  </p>
                </div>
              </div>
            </div>
          </div>

          <DialogFooter>
            <button
              type="button"
              onClick={() => active && respond(active.promptId ?? '', false)}
              disabled={pendingDecision != null}
              className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50"
            >
              {pendingDecision === 'deny' ? 'Denying…' : 'Deny'}
            </button>
            <button
              type="button"
              onClick={() => active && respond(active.promptId ?? '', true)}
              disabled={pendingDecision != null}
              className={cn(
                'rounded-md border px-4 py-2 text-sm transition-all',
                'border-warning/30 bg-warning/10 hover:bg-warning/20',
                'disabled:cursor-not-allowed disabled:opacity-50',
              )}
            >
              {pendingDecision === 'allow' ? 'Allowing…' : 'Allow takeover'}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {children}
    </RuntimeHandoffProvider>
  )
}

// useHandoffState watches the current handoff state via the Root RPC.
function useHandoffState(
  service: RootResourceService | null,
): RuntimeHandoffState | null {
  const watchFn = useCallback(
    (_: WatchRuntimeHandoffRequest, signal: AbortSignal) => {
      if (!service) return null
      return service.WatchRuntimeHandoff({}, signal)
    },
    [service],
  )
  const resp: WatchRuntimeHandoffResponse | null = useWatchStateRpc(
    watchFn,
    {},
    WatchRuntimeHandoffRequest.equals,
    WatchRuntimeHandoffResponse.equals,
  )
  return resp?.state ?? null
}

// RuntimeHandoffBanner renders the "Runtime handed off" banner with a
// Reclaim action that re-binds the listener socket.
function RuntimeHandoffBanner({
  handoff,
  service,
}: {
  handoff: RuntimeHandoffState
  service: RootResourceService | null
}) {
  const [reclaiming, setReclaiming] = useState(false)
  const onReclaim = useCallback(() => {
    if (!service) return
    setReclaiming(true)
    service
      .ReclaimRuntime({})
      .catch((err) => {
        toast.error('Reclaim failed', { description: String(err) })
      })
      .finally(() => setReclaiming(false))
  }, [service])

  const requesterName = handoff.requesterName?.trim() || 'a local process'
  const socketPath = handoff.socketPath || ''

  return (
    <div
      data-slot="runtime-handoff-banner"
      className="border-warning/20 bg-warning/5 flex w-full flex-wrap items-center justify-between gap-3 border-b px-3 py-1.5"
    >
      <div className="flex min-w-0 flex-1 items-start gap-2">
        <LuTriangleAlert className="text-warning size-3.5 shrink-0" />
        <div className="min-w-0">
          <p className="text-foreground/80 text-xs font-medium select-none">
            Runtime handed off
          </p>
          <p className="text-foreground-alt/60 mt-0.5 text-xs">
            {requesterName} is running against{' '}
            <code className="bg-foreground/5 rounded px-1 py-0.5 font-mono text-[10px]">
              {socketPath}
            </code>
            . Runtime actions are disabled until you reclaim.
          </p>
        </div>
      </div>
      <button
        type="button"
        onClick={onReclaim}
        disabled={reclaiming}
        className={cn(
          'h-7 shrink-0 rounded-md border px-3 text-xs font-medium transition-all duration-150',
          'border-warning/30 bg-warning/10 hover:border-warning/50 hover:bg-warning/15',
          'text-foreground disabled:cursor-not-allowed disabled:opacity-50',
        )}
      >
        {reclaiming ? 'Reclaiming…' : 'Reclaim runtime'}
      </button>
    </div>
  )
}

import { useCallback, useMemo, useRef, useState, type ReactNode } from 'react'
import { isDesktop } from '@aptre/bldr'
import { useWatchStateRpc } from '@aptre/bldr-react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

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

import { Button } from '@s4wave/web/ui/button.js'
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

  return (
    <RuntimeHandoffProvider state={handoff}>
      {handoff?.active && (
        <RuntimeHandoffBanner handoff={handoff} service={service} />
      )}

      <Dialog open={active != null} onOpenChange={onOpenChange}>
        <DialogContent showCloseButton={false} className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Allow command-line takeover?</DialogTitle>
            <DialogDescription>
              A local process is asking the Spacewave desktop app to hand off
              the shared runtime socket. If you allow it, this window will enter
              a handed-off state until you reclaim the runtime.
            </DialogDescription>
          </DialogHeader>

          <div className="text-foreground-alt flex flex-col gap-2 text-sm">
            <div className="flex items-start gap-2">
              <span className="text-foreground-alt/60 shrink-0 font-medium">
                Requesting runtime
              </span>
              <code className="bg-foreground/5 rounded px-1.5 py-0.5 font-mono text-xs">
                {active?.requesterName || 'spacewave serve'}
              </code>
            </div>
            <div className="flex items-start gap-2">
              <span className="text-foreground-alt/60 shrink-0 font-medium">
                Socket path
              </span>
              <code className="bg-foreground/5 truncate rounded px-1.5 py-0.5 font-mono text-xs">
                {active?.socketPath || ''}
              </code>
            </div>
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              disabled={pendingDecision != null}
              onClick={() => active && respond(active.promptId ?? '', false)}
            >
              Deny
            </Button>
            <Button
              variant="default"
              disabled={pendingDecision != null}
              onClick={() => active && respond(active.promptId ?? '', true)}
            >
              Allow
            </Button>
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

  return (
    <div
      data-slot="runtime-handoff-banner"
      className={cn(
        'bg-amber-500/15 text-amber-900 dark:text-amber-200',
        'border-b border-amber-500/40',
        'flex w-full flex-wrap items-center justify-between gap-3 px-4 py-2 text-xs',
      )}
    >
      <div className="flex flex-col gap-0.5">
        <span className="font-semibold tracking-tight">Runtime handed off</span>
        <span className="text-foreground-alt/80">
          {handoff.requesterName || 'spacewave serve'} is running against{' '}
          <code className="bg-foreground/5 rounded px-1 py-0.5 font-mono text-[0.65rem]">
            {handoff.socketPath || ''}
          </code>
          . Runtime actions are disabled until you reclaim.
        </span>
      </div>
      <Button
        variant="default"
        size="sm"
        disabled={reclaiming}
        onClick={onReclaim}
      >
        {reclaiming ? 'Reclaiming...' : 'Reclaim runtime'}
      </Button>
    </div>
  )
}

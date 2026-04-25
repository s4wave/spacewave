import { useCallback, useRef, useState } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { isDesktop } from '@aptre/bldr'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Root } from '@s4wave/sdk/root'
import {
  LauncherClient,
  type Launcher,
} from '@s4wave/core/provider/spacewave/launcher/launcher_srpc.pb.js'
import {
  UpdatePhase,
  WatchLauncherInfoRequest,
  LauncherInfo,
} from '@s4wave/core/provider/spacewave/launcher/launcher.pb.js'
import { toast } from '@s4wave/web/ui/toaster.js'

// UpdateNotifier watches the launcher update state and shows toast
// notifications when an update is staged and ready to install.
// Only active on desktop (Saucer/Electron) where the Launcher service exists.
export function UpdateNotifier({
  rootResource,
}: {
  rootResource: Resource<Root>
}) {
  if (!isDesktop) {
    return null
  }
  return <UpdateNotifierInner rootResource={rootResource} />
}

function UpdateNotifierInner({
  rootResource,
}: {
  rootResource: Resource<Root>
}) {
  const root = rootResource.value
  const prevPhaseRef = useRef<UpdatePhase | undefined>(undefined)
  const [watchDisabled, setWatchDisabled] = useState(false)

  const handleWatchError = useCallback(() => {
    setWatchDisabled(true)
  }, [])

  const watchFn = useCallback(
    (_: WatchLauncherInfoRequest, signal: AbortSignal) => {
      if (!root || watchDisabled) return null
      const svc: Launcher = new LauncherClient(root.client)
      return svc.WatchLauncherInfo({}, signal)
    },
    [root, watchDisabled],
  )

  // useWatchStateRpc returns T | null directly.
  const info: LauncherInfo | null = useWatchStateRpc(
    watchFn,
    {},
    WatchLauncherInfoRequest.equals,
    LauncherInfo.equals,
    { errorCb: handleWatchError },
  )

  const phase = info?.updateState?.phase

  // show toast when phase transitions to STAGED
  if (phase !== prevPhaseRef.current) {
    if (phase === UpdatePhase.UpdatePhase_STAGED) {
      const version = info?.updateState?.version || 'new version'
      toast('Update ready', {
        description: `Version ${version} is ready to install.`,
        duration: Infinity,
        action: {
          label: 'Restart now',
          onClick: () => {
            if (!root) return
            const svc: Launcher = new LauncherClient(root.client)
            svc.ApplyUpdate({}).catch((err) => {
              toast.error('Update failed', {
                description: String(err),
              })
            })
          },
        },
      })
    } else if (phase === UpdatePhase.UpdatePhase_ERROR) {
      const msg = info?.updateState?.errorMessage || 'Unknown error'
      toast.error('Update error', { description: msg })
    }
    prevPhaseRef.current = phase
  }

  return null
}

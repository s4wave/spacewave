import {
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useState,
} from 'react'
import { useBldrContext, useWatchStateRpc } from '@aptre/bldr-react'
import { isDesktop } from '@aptre/bldr'
import { Client as SRPCClient } from 'starpc'

import {
  LauncherClient,
  LauncherServiceName,
} from '@s4wave/core/provider/spacewave/launcher/launcher_srpc.pb.js'
import {
  UpdatePhase,
  WatchLauncherInfoRequest,
  LauncherInfo,
} from '@s4wave/core/provider/spacewave/launcher/launcher.pb.js'
import { toast } from '@s4wave/web/ui/toaster.js'

const launcherServiceId = 'plugin/spacewave-launcher/' + LauncherServiceName

/** UpdateNotifier announces staged launcher updates on desktop. */
export function UpdateNotifier() {
  return isDesktop ? <UpdateNotifierInner /> : null
}

function UpdateNotifierInner() {
  // Launcher runs in its own plugin, reached through the WebView host.
  const bldrContext = useBldrContext()
  const webDocument = bldrContext?.webDocument ?? null
  const webViewUuid = bldrContext?.webView?.getUuid() ?? null
  const launcher = useMemo(() => {
    if (!webDocument || !webViewUuid) {
      return null
    }
    const rpcClient = new SRPCClient(
      webDocument.buildWebViewHostOpenStream(webViewUuid),
    )
    return new LauncherClient(rpcClient, { service: launcherServiceId })
  }, [webDocument, webViewUuid])

  // Stop watching when the desktop host cannot provide launcher state.
  const [watchDisabled, setWatchDisabled] = useState(false)
  const handleWatchError = useCallback(() => {
    setWatchDisabled(true)
  }, [])
  const watchFn = useCallback(
    (req: WatchLauncherInfoRequest, signal: AbortSignal) =>
      !watchDisabled && launcher
        ? launcher.WatchLauncherInfo(req, signal)
        : null,
    [launcher, watchDisabled],
  )
  const info: LauncherInfo | null = useWatchStateRpc(
    watchFn,
    {},
    WatchLauncherInfoRequest.equals,
    LauncherInfo.equals,
    { errorCb: handleWatchError },
  )

  // Restart actions use the same launcher connection as the update watch.
  const announcePhase = useEffectEvent((phase: UpdatePhase | undefined) => {
    if (phase === UpdatePhase.STAGED) {
      const version = info?.updateState?.version || 'new version'
      toast('Update ready', {
        description: `Version ${version} is ready to install.`,
        duration: Infinity,
        action: {
          label: 'Restart now',
          onClick: () => {
            if (!launcher) {
              return
            }
            launcher.ApplyUpdate({}).catch((err) => {
              toast.error('Update failed', {
                description: String(err),
              })
            })
          },
        },
      })
    } else if (phase === UpdatePhase.ERROR) {
      const msg = info?.updateState?.errorMessage || 'Unknown error'
      toast.error('Update error', { description: msg })
    }
  })

  // Announce each phase transition once.
  const phase = info?.updateState?.phase
  useEffect(() => {
    announcePhase(phase)
  }, [phase])

  return null
}

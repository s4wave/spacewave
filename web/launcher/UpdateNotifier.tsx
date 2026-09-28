import {
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useState,
} from 'react'
import { useBldrContext, useWatchStateRpc } from '@aptre/bldr-react'
import { applyElectronAppUpdate, isDesktop } from '@aptre/bldr'
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

  // Announce each target using its own staged state and action.
  const announcePhase = useEffectEvent(
    (
      appPhase: UpdatePhase | undefined,
      daemonPhase: UpdatePhase | undefined,
    ) => {
      if (appPhase === UpdatePhase.STAGED) {
        const version = info?.updateState?.version || 'new version'
        toast('App update ready', {
          description: `Spacewave app ${version} is ready to install. The shared daemon will keep running.`,
          duration: Infinity,
          action: info?.updateState?.stagedPath?.endsWith('.app')
            ? {
                label: 'Update app',
                onClick: () => {
                  if (!launcher || !webViewUuid) {
                    return
                  }
                  applyElectronAppUpdate(webViewUuid).catch((err) => {
                    toast.error('Update failed', {
                      description: String(err),
                    })
                  })
                },
              }
            : undefined,
        })
      } else if (appPhase === UpdatePhase.ERROR) {
        const msg = info?.updateState?.errorMessage || 'Unknown error'
        toast.error('App update error', { description: msg })
      }

      if (daemonPhase === UpdatePhase.STAGED) {
        toast('Daemon update available', {
          description: `The ${info?.daemonUpdateState?.artifactManifestId || 'daemon'} artifact is staged. Daemon replacement is a separate operation.`,
          duration: Infinity,
        })
      }
    },
  )

  // Announce version changes even if the target remains in the staged phase.
  const appPhase = info?.updateState?.phase
  const appVersion = info?.updateState?.version
  const daemonPhase = info?.daemonUpdateState?.phase
  const daemonVersion = info?.daemonUpdateState?.version
  useEffect(() => {
    announcePhase(appPhase, daemonPhase)
  }, [appPhase, appVersion, daemonPhase, daemonVersion])

  return null
}

import {
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useState,
} from 'react'
import { useBldrContext, useWatchStateRpc } from '@aptre/bldr-react'
import {
  applyElectronAppUpdate,
  installedElectronAppVersion,
  isDesktop,
  UpdateTarget,
} from '@aptre/bldr'
import { Client as SRPCClient } from 'starpc'

import {
  LauncherClient,
  LauncherServiceName,
} from '@s4wave/core/provider/spacewave/launcher/launcher_srpc.pb.js'
import {
  DaemonUpdateWait,
  LauncherInfo,
  UpdatePhase,
  WatchLauncherInfoRequest,
} from '@s4wave/core/provider/spacewave/launcher/launcher.pb.js'
import { toast } from '@s4wave/web/ui/toaster.js'

const launcherServiceId = 'plugin/spacewave-launcher/' + LauncherServiceName

// daemonToastId keeps one daemon update toast current across its phases.
const daemonToastId = 'daemon-update'

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

  // The installed app version suppresses a staged release it already runs.
  // An empty version means the installed app is unknown.
  const [installedVersion, setInstalledVersion] = useState<string | null>(null)
  useEffect(() => {
    installedElectronAppVersion()
      .catch(() => '')
      .then(setInstalledVersion)
  }, [])

  // Announce the installed-app target with Electron's own update action.
  const announceApp = useEffectEvent((appPhase: UpdatePhase | undefined) => {
    if (appPhase === UpdatePhase.STAGED) {
      if (info?.updateState?.version === installedVersion) {
        return
      }
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
  })

  // Announce the daemon target in one toast that follows its wait.
  const announceDaemon = useEffectEvent(
    (daemonPhase: UpdatePhase | undefined) => {
      if (daemonPhase === UpdatePhase.STAGED) {
        toast('Daemon update available', {
          id: daemonToastId,
          description: `The ${info?.daemonUpdateState?.artifactManifestId || 'daemon'} artifact is ready. The daemon will restart after its other clients and services finish.`,
          duration: Infinity,
          action: {
            label: 'Update daemon',
            onClick: () => {
              launcher
                ?.ApplyUpdate({ target: UpdateTarget.DAEMON })
                .catch((err) => {
                  toast.error('Daemon update failed', {
                    id: daemonToastId,
                    description: String(err),
                  })
                })
            },
          },
        })
      } else if (daemonPhase === UpdatePhase.APPLYING) {
        const wait = info?.daemonUpdateWait
        const busy = describeDaemonWait(wait)
        toast('Daemon update accepted', {
          id: daemonToastId,
          description:
            wait?.restartNow || !busy
              ? 'Restarting the daemon. Clients will reconnect.'
              : `Waiting for ${busy} to finish before restarting.`,
          duration: Infinity,
          action:
            wait?.restartNow || !busy
              ? undefined
              : {
                  label: 'Restart now',
                  onClick: () => {
                    launcher?.RestartDaemonUpdateNow({}).catch((err) => {
                      toast.error('Daemon restart failed', {
                        id: daemonToastId,
                        description: String(err),
                      })
                    })
                  },
                },
        })
      } else if (daemonPhase === UpdatePhase.ERROR) {
        toast.error('Daemon update failed', {
          id: daemonToastId,
          description: info?.daemonUpdateState?.errorMessage || 'Unknown error',
        })
      }
    },
  )

  // Announce version changes even if the target remains in the staged phase.
  const appPhase = info?.updateState?.phase
  const appVersion = info?.updateState?.version
  useEffect(() => {
    if (installedVersion !== null) {
      announceApp(appPhase)
    }
  }, [appPhase, appVersion, installedVersion])

  const daemonPhase = info?.daemonUpdateState?.phase
  const daemonVersion = info?.daemonUpdateState?.version
  const otherWork = info?.daemonUpdateWait?.otherWork?.join('\n')
  const restartNow = info?.daemonUpdateWait?.restartNow
  useEffect(() => {
    announceDaemon(daemonPhase)
  }, [daemonPhase, daemonVersion, otherWork, restartNow])

  return null
}

// daemonWaitList joins the names of the work a daemon update waits for.
const daemonWaitList = new Intl.ListFormat('en', { type: 'conjunction' })

// describeDaemonWait names the other work an accepted daemon update waits for,
// or returns an empty string when nothing else holds the daemon.
function describeDaemonWait(wait: DaemonUpdateWait | undefined): string {
  return daemonWaitList.format(wait?.otherWork ?? [])
}

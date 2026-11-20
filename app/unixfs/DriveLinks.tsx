import {
  createContext,
  use,
  useCallback,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { LuCloud, LuFolderSync, LuRotateCcw } from 'react-icons/lu'

import { useWatchStateRpc } from '@aptre/bldr-react'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  WatchDriveLinksRequest,
  WatchDriveLinksResponse,
} from '@s4wave/sdk/space/space.pb.js'
import { SessionContext, SpaceContext } from '@s4wave/web/contexts/contexts.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { useSessionSyncSummary } from '@s4wave/app/session/SessionSyncStatusContext.js'

import {
  cloudRow,
  driveLinkRow,
  type DriveLinkRow,
  type DriveLinksView,
} from './drive-links.js'

const DriveLinksContext = createContext<DriveLinksView | null>(null)

/**
 * useDriveLinks returns the folders linked into the open Drive, or null when
 * none is: the strip draws them, and a file that cannot be read names them.
 */
export function useDriveLinks(): DriveLinksView | null {
  return use(DriveLinksContext)
}

/** DriveLinksViewProvider provides a view of the linked folders as it is. */
export function DriveLinksViewProvider({
  view,
  children,
}: {
  view: DriveLinksView | null
  children: ReactNode
}) {
  return <DriveLinksContext value={view}>{children}</DriveLinksContext>
}

/**
 * DriveLinksProvider watches the folders linked into the Drive at objectKey
 * and provides their view below it. A Drive with no linked folder provides
 * null.
 */
export function DriveLinksProvider({
  objectKey,
  children,
}: {
  objectKey: string
  children: ReactNode
}) {
  const space = useResourceValue(SpaceContext.useContext())
  const session = useResourceValue(SessionContext.useContext())
  const { isCloud } = useSessionInfo(session)
  const { uploadHold } = useSessionSyncSummary()
  const resp = useWatchStateRpc(
    useCallback(
      (req: WatchDriveLinksRequest, signal: AbortSignal) =>
        space?.watchDriveLinks(req.objectKey ?? '', signal) ?? null,
      [space],
    ),
    { objectKey },
    WatchDriveLinksRequest.equals,
    WatchDriveLinksResponse.equals,
  )
  const view = useMemo((): DriveLinksView | null => {
    const links = resp?.links ?? []
    if (!links.length) return null
    return {
      rows: links.map(driveLinkRow),
      cloud: isCloud ? cloudRow(uploadHold) : null,
    }
  }, [isCloud, resp, uploadHold])
  return <DriveLinksContext value={view}>{children}</DriveLinksContext>
}

/**
 * DriveLinksHeader draws the strip of the open Drive's linked folders, with
 * Reset on a rejected link that runs on this device.
 */
export function DriveLinksHeader() {
  const view = useDriveLinks()
  const space = useResourceValue(SpaceContext.useContext())
  const [resetting, setResetting] = useState<string | null>(null)
  const reset = useCallback(
    async (folder: string) => {
      if (!space) return
      setResetting(folder)
      try {
        await space.resetFolderRoot(folder)
      } catch (err) {
        toast.error('Could not reset the folder', {
          description: err instanceof Error ? err.message : String(err),
        })
      } finally {
        setResetting(null)
      }
    },
    [space],
  )
  if (!view) return null
  return (
    <DriveLinksStrip
      view={view}
      resetting={resetting}
      onReset={(folder) => void reset(folder)}
    />
  )
}

/** DriveLinksStrip lists the linking Devices and Spacewave Cloud. */
export function DriveLinksStrip({
  view,
  resetting,
  onReset,
}: {
  view: DriveLinksView
  resetting: string | null
  onReset: (folder: string) => void
}) {
  return (
    <section
      aria-label="Linked folders"
      data-testid="drive-links"
      className="border-foreground/8 bg-background/30 divide-foreground/6 divide-y border-b"
    >
      {view.rows.map((row) => (
        <DriveLinkLine
          key={row.deviceKey + row.path}
          icon={<LuFolderSync aria-hidden className="size-3.5" />}
          name={row.name}
          where={row.path}
          label={row.label}
          detail={row.detail}
          tone={toneOf(row)}
        >
          {row.folder !== undefined && (
            <Button
              size="sm"
              variant="outline"
              className="shrink-0"
              disabled={resetting === row.folder}
              onClick={() => onReset(row.folder ?? '')}
            >
              <LuRotateCcw aria-hidden className="size-3" />
              Reset
            </Button>
          )}
        </DriveLinkLine>
      ))}
      {view.cloud && (
        <DriveLinkLine
          icon={<LuCloud aria-hidden className="size-3.5" />}
          name="Spacewave Cloud"
          label={view.cloud.label}
          detail={view.cloud.detail}
          tone={view.cloud.held ? 'error' : 'idle'}
        />
      )}
    </section>
  )
}

function toneOf(row: DriveLinkRow): 'idle' | 'active' | 'error' {
  if (row.state === 'rejected') return 'error'
  if (row.state === 'indexing') return 'active'
  return 'idle'
}

function DriveLinkLine({
  icon,
  name,
  where,
  label,
  detail,
  tone,
  children,
}: {
  icon: ReactNode
  name: string
  where?: string
  label: string
  detail: string
  tone: 'idle' | 'active' | 'error'
  children?: ReactNode
}) {
  return (
    <div className="viewer-inset-x flex items-center gap-2 py-1.5 text-xs">
      <span className="text-foreground-alt/70 shrink-0">{icon}</span>
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 gap-1.5">
          <span className="text-foreground truncate font-medium">{name}</span>
          {where && (
            <span className="text-foreground-alt/70 truncate font-mono">
              {where}
            </span>
          )}
        </span>
        <span className="block">
          <span
            className={cn(
              tone === 'error' && 'text-destructive',
              tone === 'active' && 'text-foreground',
              tone === 'idle' && 'text-foreground-alt/80',
            )}
          >
            {label}
          </span>
          {detail && (
            <span className="text-foreground-alt/60"> · {detail}</span>
          )}
        </span>
      </span>
      {children}
    </div>
  )
}

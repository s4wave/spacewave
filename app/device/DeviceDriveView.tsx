import { useCallback, useState } from 'react'
import { LuArrowLeft, LuFolder, LuPencil, LuWifiOff } from 'react-icons/lu'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { UnixFSBrowser } from '@s4wave/app/unixfs/UnixFSBrowser.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'
import {
  ViewerStatus,
  type ViewerStatusProps,
} from '@s4wave/web/object/ViewerStatus.js'
import { HistoryRouter } from '@s4wave/web/router/HistoryRouter.js'
import type { To } from '@s4wave/web/router/router.js'

/** DeviceDriveBody is the state drawn by DeviceDrive.tla. */
export type DeviceDriveBody =
  | { kind: 'opening' }
  | { kind: 'handle-failed'; retry: () => void }
  | { kind: 'offline' }
  | { kind: 'connecting' }
  | { kind: 'root-failed'; detail?: string; retry: () => void }
  | {
      kind: 'files'
      root: Resource<FSHandle>
      editing: boolean
      enableEditing?: () => void
    }

interface DeviceDriveViewProps {
  name: string
  id: string
  body: DeviceDriveBody
  back: () => void
}

/** driveStatus presents the waiting and recovery states of a Device folder. */
function driveStatus(
  body: Exclude<DeviceDriveBody, { kind: 'files' }>,
): ViewerStatusProps {
  switch (body.kind) {
    case 'opening':
      return { state: 'waiting', what: 'the Device', rows: 0 }
    case 'handle-failed':
      return {
        state: 'failed',
        message: 'Could not open the Device.',
        retry: body.retry,
      }
    case 'offline':
      return {
        state: 'empty',
        message: 'Device offline. The folder opens when it returns.',
        icon: <LuWifiOff />,
      }
    case 'connecting':
      return { state: 'waiting', what: 'the Device folder', rows: 0 }
    case 'root-failed':
      return {
        state: 'failed',
        message: body.detail || 'The folder is unavailable.',
        retry: body.retry,
      }
  }
}

const pressClass =
  'focus-ring h-viewer-press min-w-viewer-press flex shrink-0 items-center justify-center gap-1 rounded px-1 text-xs hover:bg-foreground/5 pointer-coarse:min-h-11 pointer-coarse:min-w-11 [&_svg]:size-3.5'

/** DeviceDriveView draws the root state and navigates within the supplied handle. */
export function DeviceDriveView({
  name,
  id,
  body,
  back,
}: DeviceDriveViewProps) {
  // Local history keeps folder paths within this Device viewer.
  const [path, setPath] = useState('/')
  const navigate = useCallback((to: To) => setPath(to.path), [])

  // Loading, offline, and failed connections have no live file actions.
  if (body.kind !== 'files') {
    return (
      <div className="bg-background-primary flex h-full min-h-0 flex-col">
        <ViewerBar
          icon={<LuFolder />}
          title={name}
          actions={[
            {
              id: 'device',
              label: 'Back to Device',
              icon: <LuArrowLeft />,
              onClick: back,
            },
          ]}
        />
        <ViewerStatus {...driveStatus(body)} />
      </div>
    )
  }

  // Keep the Device and edit presses in the file browser's navigator.
  const actions = (
    <>
      <button
        type="button"
        className={pressClass}
        title="Back to Device"
        aria-label="Back to Device"
        onClick={back}
      >
        <LuArrowLeft aria-hidden />
      </button>
      {body.enableEditing && (
        <button
          type="button"
          className={pressClass}
          title="Enable editing"
          aria-label="Enable editing"
          onClick={body.enableEditing}
        >
          <LuPencil aria-hidden />
          <span aria-hidden className="@pane-md:inline hidden">
            Enable editing
          </span>
        </button>
      )}
    </>
  )
  return (
    <HistoryRouter path={path} onNavigate={navigate}>
      <UnixFSBrowser
        unixfsId={id}
        rootHandle={body.root}
        basePath="/"
        currentPath={path}
        editing={body.editing}
        actions={actions}
      />
    </HistoryRouter>
  )
}
